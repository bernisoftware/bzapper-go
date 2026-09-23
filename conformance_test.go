package bzapper

// Conformance (BRIEF §7): runs EVERY case of testdata/conformance/cases.json
// against an httptest server that checks each exchange — method, path (raw
// path split on "/", each segment percent-decoded), query, JSON body,
// Authorization, X-Bzapper-Client, X-Request-Id, Idempotency-Key — and, across
// retries of the same call, that the ids repeat. Then it compares the returned
// value (or error) with `expect`.
//
// testdata/conformance/cases.json is written by clients/conformance/generate.py
// in the monorepo (do not edit by hand); the public mirror only gets clients/go,
// so the suite reads the copy.
//
// A new endpoint without a method breaks this suite: every op in cases.json
// "ops" must be in conformanceOps.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var vendoredCases = filepath.Join("testdata", "conformance", "cases.json")

type conformanceFile struct {
	APIKey     string            `json:"api_key"`
	MaxRetries int               `json:"max_retries"`
	Excluded   []string          `json:"sdk_excluded_ops"`
	Ops        []string          `json:"ops"`
	Cases      []conformanceCase `json:"cases"`
	Signatures []struct {
		ID        string `json:"id"`
		Secret    string `json:"secret"`
		Body      string `json:"body"`
		Signature string `json:"signature"`
		Valid     bool   `json:"valid"`
	} `json:"signatures"`
}

type conformanceCase struct {
	ID   string `json:"id"`
	Op   string `json:"op"`
	Args struct {
		Path  map[string]json.RawMessage `json:"path"`
		Query map[string]json.RawMessage `json:"query"`
		Body  json.RawMessage            `json:"body"`
	} `json:"args"`
	Multipart bool              `json:"multipart"`
	Options   map[string]string `json:"options"`
	Exchanges []struct {
		Retry   *bool `json:"retry"`
		Request struct {
			Method  string            `json:"method"`
			Path    string            `json:"path"`
			Query   map[string]string `json:"query"`
			Body    json.RawMessage   `json:"body"`
			Headers map[string]string `json:"headers"`
		} `json:"request"`
		Response fakeResponse `json:"response"`
	} `json:"exchanges"`
	Expect map[string]json.RawMessage `json:"expect"`
}

type expectedError struct {
	Type          string   `json:"type"`
	Code          string   `json:"code"`
	Status        *int     `json:"status"`
	RequestID     *string  `json:"request_id"`
	RetryAfter    *float64 `json:"retry_after"`
	RequiredScope *string  `json:"required_scope"`
}

func loadConformance(t *testing.T) *conformanceFile {
	t.Helper()
	data, err := os.ReadFile(vendoredCases)
	if err != nil {
		t.Fatalf("read %s: %v", vendoredCases, err)
	}
	var f conformanceFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("%s: %v", vendoredCases, err)
	}
	return &f
}

// ── args: the neutral case args, converted by the table into Go calls ────────

// caseArgs gives typed access to args.path/query/body and records what was
// used: an arg the SDK call did not consume fails the case (a new parameter in
// the spec without a parameter in the SDK breaks the build).
type caseArgs struct {
	t    *testing.T
	c    *conformanceCase
	used map[string]bool
}

func (a *caseArgs) raw(where, name string) (json.RawMessage, bool) {
	var m map[string]json.RawMessage
	if where == "path" {
		m = a.c.Args.Path
	} else {
		m = a.c.Args.Query
	}
	v, ok := m[name]
	a.used[where+"."+name] = true
	return v, ok
}

// path reads a path parameter (must exist).
func (a *caseArgs) path(name string) string {
	a.t.Helper()
	v, ok := a.raw("path", name)
	if !ok {
		a.t.Fatalf("args.path has no %q", name)
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		a.t.Fatalf("args.path.%s: %v", name, err)
	}
	return s
}

// q reads a query parameter as string ("" when absent).
func (a *caseArgs) q(name string) string {
	a.t.Helper()
	v, ok := a.raw("query", name)
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return string(v) // numbers/booleans as written
}

func (a *caseArgs) qInt(name string) int {
	a.t.Helper()
	s := a.q(name)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		a.t.Fatalf("args.query.%s not an integer: %q", name, s)
	}
	return n
}

func (a *caseArgs) qBool(name string) bool {
	return a.q(name) == "true"
}

func (a *caseArgs) qBoolPtr(name string) *bool {
	if _, ok := a.c.Args.Query[name]; !ok {
		a.used["query."+name] = true
		return nil
	}
	v := a.qBool(name)
	return &v
}

func (a *caseArgs) qList(name string) []string {
	a.t.Helper()
	v, ok := a.raw("query", name)
	if !ok {
		return nil
	}
	var out []string
	if err := json.Unmarshal(v, &out); err != nil {
		a.t.Fatalf("args.query.%s not a list: %s", name, v)
	}
	return out
}

// body decodes args.body into T, refusing fields T does not have.
func body[T any](a *caseArgs) T {
	a.t.Helper()
	a.used["body"] = true
	var out T
	if len(a.c.Args.Body) == 0 || string(a.c.Args.Body) == "null" {
		return out
	}
	dec := json.NewDecoder(bytes.NewReader(a.c.Args.Body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		a.t.Fatalf("args.body → %T: %v", out, err)
	}
	return out
}

// field reads one field of a simple body, refusing other fields.
func (a *caseArgs) fields(names ...string) map[string]json.RawMessage {
	a.t.Helper()
	a.used["body"] = true
	m := map[string]json.RawMessage{}
	if len(a.c.Args.Body) > 0 && string(a.c.Args.Body) != "null" {
		if err := json.Unmarshal(a.c.Args.Body, &m); err != nil {
			a.t.Fatalf("args.body: %v", err)
		}
	}
	for k := range m {
		if !containsString(names, k) {
			a.t.Fatalf("args.body.%s has no SDK parameter", k)
		}
	}
	return m
}

func (a *caseArgs) bodyStr(name string) string {
	a.t.Helper()
	var s string
	if v, ok := a.fields(name)[name]; ok {
		if err := json.Unmarshal(v, &s); err != nil {
			a.t.Fatalf("args.body.%s: %v", name, err)
		}
	}
	return s
}

// upload converts the multipart args into an UploadFile.
func (a *caseArgs) upload() UploadFile {
	a.t.Helper()
	a.used["body"] = true
	var b struct {
		Fields map[string]string `json:"fields"`
		File   struct {
			Filename      string `json:"filename"`
			ContentType   string `json:"content_type"`
			ContentBase64 string `json:"content_base64"`
		} `json:"file"`
	}
	if err := json.Unmarshal(a.c.Args.Body, &b); err != nil {
		a.t.Fatalf("multipart args: %v", err)
	}
	if len(b.Fields) > 0 {
		a.t.Fatalf("multipart fields %v have no SDK parameter", b.Fields)
	}
	return UploadFile{Filename: b.File.Filename, ContentType: b.File.ContentType, Content: []byte(b.File.ContentBase64)}
}

// idem is the caller's idempotency key of the case (options.idempotency_key).
func (a *caseArgs) idem() string { return a.c.Options["idempotency_key"] }

// finish fails when an arg was not consumed by the SDK call.
func (a *caseArgs) finish() {
	a.t.Helper()
	for k := range a.c.Args.Path {
		if !a.used["path."+k] {
			a.t.Errorf("args.path.%s has no SDK parameter in the table", k)
		}
	}
	for k := range a.c.Args.Query {
		if !a.used["query."+k] {
			a.t.Errorf("args.query.%s has no SDK parameter in the table", k)
		}
	}
	if len(a.c.Args.Body) > 0 && string(a.c.Args.Body) != "null" && !a.used["body"] {
		a.t.Errorf("args.body has no SDK parameter in the table")
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func ret[T any](v T, err error) (any, error) { return v, err }
func none(err error) (any, error)            { return nil, err }

type opFunc func(ctx context.Context, c *Client, p *PartnerClient, a *caseArgs) (any, error)

// conformanceOps maps each operationId to the SDK call. Existing methods whose
// Go name differs from the operationId (ListKeys ← listMyKeys, …) are what the
// table points to.
var conformanceOps = map[string]opFunc{
	// --- messages ---
	"sendText": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		p := body[SendTextParams](a)
		p.IdempotencyKey = a.idem()
		return ret(c.SendText(ctx, p))
	},
	"sendOTP": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		p := body[SendOTPParams](a)
		p.IdempotencyKey = a.idem()
		return ret(c.SendOTP(ctx, p))
	},
	"sendImage": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendImage(ctx, body[SendMediaParams](a)))
	},
	"sendVideo": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendVideo(ctx, body[SendMediaParams](a)))
	},
	"sendDocument": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendDocument(ctx, body[SendMediaParams](a)))
	},
	"sendAudio": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendAudio(ctx, body[SendMediaParams](a)))
	},
	"sendSticker": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendSticker(ctx, body[SendMediaParams](a)))
	},
	"sendLocation": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendLocation(ctx, body[SendLocationParams](a)))
	},
	"sendContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendContact(ctx, body[SendContactParams](a)))
	},
	"sendPoll": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendPoll(ctx, body[SendPollParams](a)))
	},
	"sendReaction": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendReaction(ctx, body[SendReactionParams](a)))
	},
	"sendButtons": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendButtons(ctx, body[SendButtonsParams](a)))
	},
	"sendList": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SendList(ctx, body[SendListParams](a)))
	},
	"listScheduled": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListScheduledWithParams(ctx, ListScheduledParams{Limit: a.qInt("limit")}))
	},
	"cancelScheduled": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.CancelScheduled(ctx, a.path("id")))
	},
	"editMessage": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.EditMessage(ctx, a.path("id"), a.bodyStr("text")))
	},
	"revokeMessage": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.RevokeMessage(ctx, a.path("id"), a.qBool("for_everyone")))
	},
	"forwardMessage": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ForwardMessage(ctx, body[ForwardMessageParams](a)))
	},
	"markRead": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.MarkRead(ctx, a.path("id"), body[MarkReadParams](a)))
	},
	"presenceChat": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.PresenceChat(ctx, body[PresenceChatParams](a)))
	},

	// --- instances ---
	"listInstances": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListInstances(ctx, ListInstancesParams{ProjectID: a.q("project_id"), Archived: a.q("archived")}))
	},
	"createInstance": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateInstance(ctx, body[CreateInstanceParams](a)))
	},
	"getInstance": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetInstance(ctx, a.path("id")))
	},
	"deleteInstance": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DeleteInstance(ctx, a.path("id")))
	},
	"connectInstance": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ConnectInstance(ctx, a.path("id"), ConnectMethod(a.q("method"))))
	},
	"disconnectInstance": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DisconnectInstance(ctx, a.path("id")))
	},
	"logoutInstance": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.LogoutInstance(ctx, a.path("id")))
	},
	"clearInstanceSession": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.ClearInstanceSession(ctx, a.path("id")))
	},
	"archiveInstance": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.ArchiveInstance(ctx, a.path("id")))
	},
	"unarchiveInstance": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.UnarchiveInstance(ctx, a.path("id")))
	},
	"setInstanceProxy": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.SetInstanceProxy(ctx, a.path("id"), a.bodyStr("proxy_url")))
	},
	"setInboundFilters": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SetInboundFilters(ctx, a.path("id"), body[InboundFilters](a)))
	},
	"setProfile": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SetProfile(ctx, a.path("id"), body[SetProfileParams](a)))
	},
	"setPrivacy": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.SetPrivacy(ctx, a.path("id"), body[PrivacyParams](a)))
	},
	"getOfficialAccount": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetOfficialAccount(ctx))
	},
	"connectOfficialAccount": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ConnectOfficialAccount(ctx, body[ConnectOfficialAccountParams](a)))
	},
	"disconnectOfficialAccount": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DisconnectOfficialAccount(ctx))
	},
	"getHealth": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetHealth(ctx))
	},

	// --- conversations, chats, labels, block, calls ---
	"listConversations": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListConversations(ctx, a.q("instance_id")))
	},
	"conversationHistory": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ConversationHistory(ctx, a.path("jid"), ConversationHistoryParams{
			InstanceID: a.q("instance_id"), Before: a.q("before"), Limit: a.qInt("limit")}))
	},
	"archiveChat": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		b := body[chatActionArgs](a)
		return ret(c.ArchiveChat(ctx, a.path("jid"), b.InstanceID, b.On))
	},
	"pinChat": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		b := body[chatActionArgs](a)
		return ret(c.PinChat(ctx, a.path("jid"), b.InstanceID, b.On))
	},
	"markChat": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		b := body[chatActionArgs](a)
		return ret(c.MarkChat(ctx, a.path("jid"), b.InstanceID, b.On))
	},
	"muteChat": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		b := body[chatActionArgs](a)
		return none(c.MuteChat(ctx, a.path("jid"), b.InstanceID, b.On))
	},
	"applyChatLabel": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.ApplyChatLabel(ctx, a.path("jid"), body[ApplyChatLabelParams](a)))
	},
	"listLabels": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListLabels(ctx, a.q("instance_id")))
	},
	"createLabel": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateLabel(ctx, body[CreateLabelParams](a)))
	},
	"deleteLabel": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DeleteLabel(ctx, a.path("id"), a.q("instance_id")))
	},
	"blockContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.BlockContact(ctx, a.path("jid"), a.bodyStr("instance_id")))
	},
	"unblockContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.UnblockContact(ctx, a.path("jid"), a.bodyStr("instance_id")))
	},
	"getBlocklist": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetBlocklist(ctx, a.q("instance_id")))
	},
	"rejectCall": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.RejectCall(ctx, body[RejectCallParams](a)))
	},
	"offerCall": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.OfferCall(ctx, body[OfferCallParams](a)))
	},

	// --- groups ---
	"listGroups": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListGroups(ctx, a.q("instance_id")))
	},
	"createGroup": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateGroup(ctx, a.q("instance_id"), body[CreateGroupParams](a)))
	},
	"getGroup": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetGroup(ctx, a.path("jid"), a.q("instance_id")))
	},
	"updateGroup": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.UpdateGroup(ctx, a.path("jid"), a.q("instance_id"), body[UpdateGroupParams](a)))
	},
	"joinGroup": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.JoinGroup(ctx, a.q("instance_id"), body[JoinGroupParams](a)))
	},
	"previewGroupInvite": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.PreviewGroupInvite(ctx, a.q("instance_id"), body[JoinGroupParams](a)))
	},
	"updateGroupParticipants": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UpdateGroupParticipants(ctx, a.path("jid"), a.q("instance_id"), body[UpdateGroupParticipantsParams](a)))
	},
	"leaveGroup": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.LeaveGroup(ctx, a.path("jid"), a.q("instance_id")))
	},
	"groupInviteLink": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GroupInviteLink(ctx, a.path("jid"), GroupInviteLinkParams{InstanceID: a.q("instance_id"), Reset: a.qBool("reset")}))
	},
	"listJoinRequests": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListJoinRequests(ctx, a.path("jid"), a.q("instance_id")))
	},
	"updateJoinRequests": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.UpdateJoinRequests(ctx, a.path("jid"), a.q("instance_id"), body[UpdateJoinRequestsParams](a)))
	},

	// --- contacts (captured + CRM) ---
	"contactsCheck": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ContactsCheck(ctx, body[ContactsCheckParams](a)))
	},
	"listContacts": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListContacts(ctx, ListContactsParams{
			Search: a.q("search"), ProjectID: a.q("project_id"), InstanceID: a.q("instance_id"), Limit: a.qInt("limit"),
			Tags: strings.Join(a.qList("tags"), ","), TagsMatch: a.q("tags_match"), Groups: strings.Join(a.qList("groups"), ","), Status: a.q("status"),
			City: a.q("city"), State: a.q("state"), Country: a.q("country"), Zip: a.q("zip"), Document: a.q("document"),
			HasEmail: a.qBoolPtr("has_email"), LastActivityAfter: a.q("last_activity_after"),
			LastActivityBefore: a.q("last_activity_before"), CreatedAfter: a.q("created_after"),
			CreatedBefore: a.q("created_before"), Sort: a.q("sort"), Offset: a.qInt("offset"),
		}))
	},
	"createContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateContact(ctx, body[CreateContactParams](a)))
	},
	"importContacts": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ImportContacts(ctx, body[ImportContactsParams](a)))
	},
	"getContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetContact(ctx, a.path("id")))
	},
	"updateContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UpdateContact(ctx, a.path("id"), body[UpdateContactParams](a)))
	},
	"deleteContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DeleteContact(ctx, a.path("id")))
	},
	"getContactHistory": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetContactHistory(ctx, a.path("id"), ContactHistoryParams{Limit: a.qInt("limit")}))
	},
	"addContactNote": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.AddContactNote(ctx, a.path("id"), a.bodyStr("body")))
	},
	"mutateContactTags": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.MutateContactTags(ctx, a.path("id"), body[TaxonMutation](a)))
	},
	"mutateContactGroups": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.MutateContactGroups(ctx, a.path("id"), body[TaxonMutation](a)))
	},
	"optOutContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.OptOutContact(ctx, a.path("id")))
	},
	"suppressContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SuppressContact(ctx, a.path("id")))
	},
	"optInContact": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.OptInContact(ctx, a.path("id")))
	},
	"listTags": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListTags(ctx))
	},
	"createTag": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateTag(ctx, body[CreateTaxonParams](a)))
	},
	"deleteTag": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DeleteTag(ctx, a.path("id")))
	},
	"listContactGroups": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListContactGroups(ctx))
	},
	"createContactGroup": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateContactGroup(ctx, body[CreateTaxonParams](a)))
	},
	"deleteContactGroup": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DeleteContactGroup(ctx, a.path("id")))
	},
	"listSuppressions": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListSuppressions(ctx, ListSuppressionsParams{Limit: a.qInt("limit")}))
	},
	"createSuppression": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.CreateSuppression(ctx, body[CreateSuppressionParams](a)))
	},
	"deleteSuppression": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DeleteSuppression(ctx, a.q("phone")))
	},

	// --- campaigns ---
	"listCampaigns": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListCampaignsWithParams(ctx, ListCampaignsParams{Limit: a.qInt("limit")}))
	},
	"createCampaign": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateCampaign(ctx, body[CampaignCreateParams](a)))
	},
	"estimateCampaign": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.EstimateCampaign(ctx, CampaignEstimateParams{
			Recipients: a.qInt("recipients"), Pacing: a.q("pacing"), PoolID: a.q("pool_id")}))
	},
	"getCampaignEligibility": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetCampaignEligibility(ctx, CampaignEligibilityParams{PoolID: a.q("pool_id")}))
	},
	"uploadCampaignMedia": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UploadCampaignMedia(ctx, a.upload()))
	},
	"getCampaign": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetCampaign(ctx, a.path("id")))
	},
	"updateCampaign": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UpdateCampaign(ctx, a.path("id"), body[CampaignCreateParams](a)))
	},
	"listCampaignRecipients": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListCampaignRecipientsWithParams(ctx, a.path("id"), ListCampaignRecipientsParams{Limit: a.qInt("limit")}))
	},
	"addCampaignRecipients": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.AddCampaignRecipients(ctx, a.path("id"), body[CampaignRecipientsParams](a)))
	},
	"startCampaign": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.StartCampaignWithResult(ctx, a.path("id")))
	},
	"pauseCampaign": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.PauseCampaignWithResult(ctx, a.path("id")))
	},
	"resumeCampaign": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ResumeCampaignWithResult(ctx, a.path("id")))
	},
	"cancelCampaign": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CancelCampaignWithResult(ctx, a.path("id")))
	},
	"dryRunCampaign": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.DryRunCampaign(ctx, a.path("id")))
	},

	// --- pools ---
	"listPools": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListPools(ctx))
	},
	"createPool": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreatePool(ctx, body[CreatePoolParams](a)))
	},
	"getPool": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetPool(ctx, a.path("id")))
	},
	"addPoolNumber": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.AddPoolNumber(ctx, a.path("id"), a.bodyStr("instance_id")))
	},

	// --- webhooks + advisories ---
	"listWebhooks": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListWebhooks(ctx))
	},
	"createWebhook": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateWebhook(ctx, body[CreateWebhookParams](a)))
	},
	"updateWebhook": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UpdateWebhook(ctx, a.path("id"), body[UpdateWebhookParams](a)))
	},
	"deleteWebhook": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DeleteWebhook(ctx, a.path("id")))
	},
	"testWebhook": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.TestWebhook(ctx, a.path("id"), a.bodyStr("event_type")))
	},
	"listWebhookDeliveries": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.WebhookDeliveries(ctx, a.path("id"), a.qInt("limit")))
	},
	"triggerWebhookEvent": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.TriggerWebhookEvent(ctx, a.bodyStr("event_type")))
	},
	"listAdvisories": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListAdvisories(ctx))
	},
	"markAdvisoryRead": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.MarkAdvisoryRead(ctx, a.path("id")))
	},

	// --- identity, account, projects, users, keys, brand, usage ---
	"getMe": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetMe(ctx))
	},
	"updateProfile": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UpdateProfile(ctx, body[UpdateProfileParams](a)))
	},
	"updateAccount": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UpdateAccount(ctx, body[UpdateAccountParams](a)))
	},
	"listMyKeys": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListKeys(ctx))
	},
	"createMyKey": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateKey(ctx, body[CreateKeyParams](a)))
	},
	"revokeMyKey": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.RevokeKey(ctx, a.path("id")))
	},
	"rotateMyKey": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.RotateKey(ctx, a.path("id"), body[RotateKeyParams](a)))
	},
	"listProjects": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListProjects(ctx))
	},
	"createProject": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CreateProjectWithParams(ctx, body[CreateProjectParams](a)))
	},
	"getProjectsHealth": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetProjectsHealth(ctx))
	},
	"updateProject": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.UpdateProject(ctx, a.path("id"), body[UpdateProjectParams](a)))
	},
	"deleteProject": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.DeleteProject(ctx, a.path("id")))
	},
	"getProjectBrand": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetProjectBrand(ctx, a.path("id")))
	},
	"setProjectBrand": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SetProjectBrand(ctx, a.path("id"), body[BrandProfile](a)))
	},
	"uploadProjectLogo": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UploadProjectLogo(ctx, a.path("id"), a.upload()))
	},
	"listUsers": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListUsers(ctx))
	},
	"inviteUser": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.InviteUser(ctx, body[InviteUserParams](a)))
	},
	"updateUserRole": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.UpdateUserRole(ctx, a.path("id"), Role(a.bodyStr("role"))))
	},
	"removeUser": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.RemoveUser(ctx, a.path("id")))
	},
	"getBrand": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetBrand(ctx))
	},
	"setBrand": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.SetBrand(ctx, body[BrandProfile](a)))
	},
	"applyBrand": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ApplyBrand(ctx))
	},
	"uploadBrandLogo": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UploadBrandLogo(ctx, a.upload()))
	},
	"getUsage": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetUsage(ctx, GetUsageParams{From: a.q("from"), To: a.q("to")}))
	},
	"getAccountUsage": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetAccountUsage(ctx, GetUsageParams{From: a.q("from"), To: a.q("to")}))
	},

	// --- billing ---
	"getMyEntitlements": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetMyEntitlements(ctx))
	},
	"getMySubscription": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetMySubscription(ctx))
	},
	"upgradePlan": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UpgradePlan(ctx))
	},
	"cancelPlan": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CancelPlan(ctx))
	},
	"uncancelPlan": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.UncancelPlan(ctx))
	},
	"changeAddon": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ChangeAddon(ctx, body[ChangeAddonParams](a)))
	},
	"getAddonCart": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetAddonCart(ctx))
	},
	"clearAddonCart": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ClearAddonCart(ctx))
	},
	"checkoutAddonCart": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.CheckoutAddonCart(ctx, body[CheckoutAddonCartParams](a)))
	},
	"listMyInvoices": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListMyInvoices(ctx))
	},
	"payInvoice": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.PayInvoice(ctx, a.path("id")))
	},
	"getBillingConfig": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetBillingConfig(ctx))
	},
	"getPricing": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.GetPricing(ctx))
	},

	// --- bZapper Connect: customer side (Client) ---
	"listConnectedApps": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return ret(c.ListConnectedApps(ctx))
	},
	"revokeConnectedApp": func(ctx context.Context, c *Client, _ *PartnerClient, a *caseArgs) (any, error) {
		return none(c.RevokeConnectedApp(ctx, a.path("id")))
	},

	// --- bZapper Connect: partner side (PartnerClient) ---
	"getPartnerMe": func(ctx context.Context, _ *Client, p *PartnerClient, a *caseArgs) (any, error) {
		return ret(p.Me(ctx))
	},
	"createConnectSession": func(ctx context.Context, _ *Client, p *PartnerClient, a *caseArgs) (any, error) {
		return ret(p.CreateConnectSession(ctx, body[CreateConnectSessionParams](a)))
	},
	"exchangeConnectCode": func(ctx context.Context, _ *Client, p *PartnerClient, a *caseArgs) (any, error) {
		return ret(p.ExchangeCode(ctx, a.bodyStr("code")))
	},
	"listPartnerConnections": func(ctx context.Context, _ *Client, p *PartnerClient, a *caseArgs) (any, error) {
		return ret(p.ListConnections(ctx, ListConnectionsParams{ExternalID: a.q("external_id"), Status: ConnectionStatus(a.q("status"))}))
	},
	"getPartnerConnection": func(ctx context.Context, _ *Client, p *PartnerClient, a *caseArgs) (any, error) {
		return ret(p.GetConnection(ctx, a.path("id")))
	},
	"revokePartnerConnection": func(ctx context.Context, _ *Client, p *PartnerClient, a *caseArgs) (any, error) {
		return none(p.RevokeConnection(ctx, a.path("id")))
	},
	"rotatePartnerConnectionKey": func(ctx context.Context, _ *Client, p *PartnerClient, a *caseArgs) (any, error) {
		return ret(p.RotateConnectionKey(ctx, a.path("id")))
	},
}

// chatActionArgs is the ChatAction body ({instance_id, on}) of the chat toggles.
type chatActionArgs struct {
	InstanceID string `json:"instance_id"`
	On         bool   `json:"on"`
}

// errorSentinels: expect.error.type → sentinel (errors.Is). "api" has none: it
// is an *Error matching no sentinel; "argument" is ErrInvalidArgument.
var errorSentinels = map[string]error{
	"authentication":    ErrAuthentication,
	"permission_denied": ErrPermissionDenied,
	"not_found":         ErrNotFound,
	"conflict":          ErrConflict,
	"validation":        ErrValidation,
	"rate_limit":        ErrRateLimit,
	"server":            ErrServer,
	"network":           ErrNetwork,
}

var (
	clientHeaderRE = regexp.MustCompile(`^bzapper-go/` + regexp.QuoteMeta(Version) + `$`)
	requestIDRE    = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

const sentRequestID = "$sent" // expect.error.request_id: the X-Request-Id the SDK sent

// ── suite ────────────────────────────────────────────────────────────────────

func TestConformance(t *testing.T) {
	file := loadConformance(t)
	if len(file.Cases) == 0 {
		t.Fatal("cases.json has no cases")
	}
	if file.MaxRetries != DefaultMaxRetries {
		t.Fatalf("cases.json max_retries %d, SDK default %d", file.MaxRetries, DefaultMaxRetries)
	}
	srv := newFakeServer(t)
	seen := map[string]bool{}
	for i := range file.Cases {
		tc := &file.Cases[i]
		if seen[tc.ID] {
			t.Fatalf("duplicate case id: %s", tc.ID)
		}
		seen[tc.ID] = true
		t.Run(tc.ID, func(t *testing.T) { runConformanceCase(t, file, srv, tc) })
	}
}

func runConformanceCase(t *testing.T, file *conformanceFile, srv *fakeServer, tc *conformanceCase) {
	op, ok := conformanceOps[tc.Op]
	if !ok {
		t.Fatalf("op %q has no method in the Go SDK — implement it and map it in conformanceOps", tc.Op)
	}
	responses := make([]fakeResponse, len(tc.Exchanges))
	for i, ex := range tc.Exchanges {
		responses[i] = ex.Response
	}
	srv.reset(responses...)
	sleeps := &sleepLog{}
	client := New(srv.URL, file.APIKey, WithMaxRetries(file.MaxRetries))
	client.hooks.sleep = sleeps.sleep // waits disabled: the suite never sleeps
	partner := NewPartner(srv.URL, file.APIKey, WithMaxRetries(file.MaxRetries))
	partner.c.hooks.sleep = sleeps.sleep

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if k := tc.Options["idempotency_key"]; k != "" {
		ctx = ContextWithIdempotencyKey(ctx, k)
	}
	args := &caseArgs{t: t, c: tc, used: map[string]bool{}}
	result, callErr := op(ctx, client, partner, args)
	args.finish()

	requests := srv.recorded()
	checkExchanges(t, file.APIKey, tc, requests)
	checkWaits(t, tc, sleeps.all())

	if rawErr, isError := tc.Expect["error"]; isError {
		var want expectedError
		if err := json.Unmarshal(rawErr, &want); err != nil {
			t.Fatalf("expect.error: %v", err)
		}
		if callErr == nil {
			t.Fatalf("expected error %s/%s, got success: %s", want.Type, want.Code, mustJSON(result))
		}
		checkError(t, want, callErr, requests)
		return
	}
	rawResult, hasResult := tc.Expect["result"]
	if !hasResult {
		t.Fatalf("case without expect.result or expect.error")
	}
	if callErr != nil {
		t.Fatalf("unexpected error: %v", callErr)
	}
	var got, want any
	if err := json.Unmarshal(mustJSON(result), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawResult, &want); err != nil {
		t.Fatal(err)
	}
	// BRIEF §4: the existing Go methods unwrap {"data": [...]} lists.
	if _, isList := got.([]any); isList || got == nil {
		if obj, ok := want.(map[string]any); ok {
			if data, has := obj["data"]; has {
				want = data
			}
		}
	}
	if diff := jsonDiff("result", got, want); diff != "" {
		t.Errorf("%s\n  got:      %s\n  expected: %s", diff, mustJSON(got), rawResult)
	}
}

func checkExchanges(t *testing.T, apiKey string, tc *conformanceCase, requests []recorded) {
	t.Helper()
	if len(requests) != len(tc.Exchanges) {
		var got []string
		for _, r := range requests {
			got = append(got, r.Method+" "+r.Path)
		}
		t.Fatalf("%d requests, expected %d (%s)", len(requests), len(tc.Exchanges), strings.Join(got, ", "))
	}
	var previous *recorded
	for i, ex := range tc.Exchanges {
		got := requests[i]
		want := ex.Request
		where := fmt.Sprintf("exchange %d", i)

		if got.Method != want.Method {
			t.Errorf("%s: method %s, expected %s", where, got.Method, want.Method)
		}
		checkPath(t, where, got.Path, want.Path)
		checkQuery(t, where, tc, got, want.Query)

		switch {
		case tc.Multipart:
			if ct := got.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data") {
				t.Errorf("%s: Content-Type %q, expected multipart/form-data", where, ct)
			}
			var file struct {
				File struct {
					Filename string `json:"filename"`
				} `json:"file"`
			}
			_ = json.Unmarshal(tc.Args.Body, &file)
			if file.File.Filename == "" || !bytes.Contains(got.Body, []byte(file.File.Filename)) {
				t.Errorf("%s: multipart body without the filename %q", where, file.File.Filename)
			}
		case len(want.Body) == 0 || string(want.Body) == "null":
			if len(got.Body) != 0 {
				t.Errorf("%s: expected no body, got %q", where, got.Body)
			}
			if got.has("Content-Type") {
				t.Errorf("%s: Content-Type without body", where)
			}
		default:
			if ct := got.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("%s: Content-Type %q", where, ct)
			}
			var gotBody, wantBody any
			if err := json.Unmarshal(got.Body, &gotBody); err != nil {
				t.Fatalf("%s: body is not JSON (%v): %q", where, err, got.Body)
			}
			if err := json.Unmarshal(want.Body, &wantBody); err != nil {
				t.Fatal(err)
			}
			if diff := bodyDiff("body", gotBody, wantBody); diff != "" {
				t.Errorf("%s: %s\n  sent:     %s\n  expected: %s", where, diff, got.Body, want.Body)
			}
		}

		if v := got.Header.Get("Authorization"); v != "Bearer "+apiKey {
			t.Errorf("%s: Authorization %q", where, v)
		}
		if v := got.Header.Get("Accept"); v != "application/json" {
			t.Errorf("%s: Accept %q", where, v)
		}
		clientID := got.Header.Get("X-Bzapper-Client")
		if !clientHeaderRE.MatchString(clientID) {
			t.Errorf("%s: X-Bzapper-Client %q does not match %s", where, clientID, clientHeaderRE)
		}
		if ua := got.Header.Get("User-Agent"); ua != clientID {
			t.Errorf("%s: User-Agent %q, expected %q", where, ua, clientID)
		}
		if id := got.Header.Get("X-Request-Id"); !requestIDRE.MatchString(id) {
			t.Errorf("%s: X-Request-Id %q (expected uuid4 hex, 32 chars)", where, id)
		}
		if isWrite(want.Method) {
			if got.Header.Get("Idempotency-Key") == "" {
				t.Errorf("%s: write without Idempotency-Key", where)
			}
		} else if got.has("Idempotency-Key") {
			t.Errorf("%s: Idempotency-Key on %s", where, want.Method)
		}
		for name, value := range want.Headers {
			if v := got.Header.Get(name); v != value {
				t.Errorf("%s: header %s %q, expected %q", where, name, v, value)
			}
		}

		if ex.Retry == nil {
			t.Fatalf("%s: exchange without 'retry'", where)
		}
		switch {
		case i == 0:
			if *ex.Retry {
				t.Fatalf("%s: the first exchange cannot be a retry", where)
			}
		case *ex.Retry:
			if a, b := got.Header.Get("X-Request-Id"), previous.Header.Get("X-Request-Id"); a != b {
				t.Errorf("%s: retry with X-Request-Id %q ≠ %q", where, a, b)
			}
			if a, b := got.Header.Get("Idempotency-Key"), previous.Header.Get("Idempotency-Key"); a != b {
				t.Errorf("%s: retry with Idempotency-Key %q ≠ %q", where, a, b)
			}
		default:
			if got.Header.Get("X-Request-Id") == previous.Header.Get("X-Request-Id") {
				t.Errorf("%s: a new call needs a new X-Request-Id", where)
			}
			if k := got.Header.Get("Idempotency-Key"); k != "" && k == previous.Header.Get("Idempotency-Key") {
				t.Errorf("%s: a new call needs a new Idempotency-Key", where)
			}
		}
		previous = &requests[i]
	}
}

// checkPath splits the RAW path on "/", percent-decodes each segment and
// compares with the expected segments (also decoded: cases carry both
// "a%2Fb%20c" and "id 1"). The raw path must not contain a space.
func checkPath(t *testing.T, where, raw, want string) {
	t.Helper()
	if strings.ContainsAny(raw, " ") {
		t.Errorf("%s: raw path %q contains a space (not encoded)", where, raw)
	}
	gotSegs, err := decodeSegments(raw)
	if err != nil {
		t.Errorf("%s: raw path %q: %v", where, raw, err)
		return
	}
	wantSegs, err := decodeSegments(want)
	if err != nil {
		wantSegs = strings.Split(want, "/") // literal (e.g. contains a bare "%")
	}
	if strings.Join(gotSegs, "\x00") != strings.Join(wantSegs, "\x00") {
		t.Errorf("%s: path %q (segments %q), expected %q", where, raw, gotSegs, wantSegs)
	}
}

func decodeSegments(p string) ([]string, error) {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		d, err := url.PathUnescape(s)
		if err != nil {
			return nil, err
		}
		parts[i] = d
	}
	return parts, nil
}

func checkQuery(t *testing.T, where string, tc *conformanceCase, got recorded, want map[string]string) {
	t.Helper()
	for key, values := range got.Query {
		wantValue, expected := want[key]
		if !expected {
			t.Errorf("%s: unexpected query %s=%v (raw %q)", where, key, values, got.RawQuery)
			continue
		}
		if len(values) != 1 || values[0] != wantValue {
			t.Errorf("%s: query %s=%v, expected %q", where, key, values, wantValue)
		}
	}
	for key := range want {
		if _, sent := got.Query[key]; !sent {
			t.Errorf("%s: query without %s (raw %q)", where, key, got.RawQuery)
		}
	}
}

// checkWaits: one wait per retry — the previous response's Retry-After when
// present (capped at 60s), else min(8, 0.5 × 2^n)s + up to 25%.
func checkWaits(t *testing.T, tc *conformanceCase, waits []time.Duration) {
	t.Helper()
	var retries []int
	for i, ex := range tc.Exchanges {
		if ex.Retry != nil && *ex.Retry {
			retries = append(retries, i)
		}
	}
	if len(waits) != len(retries) {
		t.Fatalf("waits %v, expected %d (one per retry)", waits, len(retries))
	}
	for n, i := range retries {
		prev := tc.Exchanges[i-1].Response
		if ra := headerValue(prev.Headers, "Retry-After"); ra != "" {
			secs, err := strconv.ParseFloat(ra, 64)
			if err != nil {
				t.Fatalf("case Retry-After %q is not a number", ra)
			}
			if want := time.Duration(math.Min(secs, 60) * float64(time.Second)); waits[n] != want {
				t.Errorf("wait %d: %s, expected the Retry-After %s", n, waits[n], want)
			}
			continue
		}
		base := time.Duration(math.Min(8, 0.5*math.Pow(2, float64(n))) * float64(time.Second))
		if waits[n] < base || waits[n] > base+base/4 {
			t.Errorf("wait %d: %s, expected between %s and %s", n, waits[n], base, base+base/4)
		}
	}
}

func headerValue(headers map[string]string, name string) string {
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func checkError(t *testing.T, want expectedError, err error, requests []recorded) {
	t.Helper()
	if want.Type == "argument" {
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("expected an argument error (ErrInvalidArgument), got %T: %v", err, err)
		}
		var e *Error
		if errors.As(err, &e) {
			t.Errorf("an argument error must not be an *Error: %v", err)
		}
		return
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *bzapper.Error (%s), got %T: %v", want.Type, err, err)
	}
	if string(e.Type) != want.Type {
		t.Errorf("Type %q, expected %q", e.Type, want.Type)
	}
	if _, known := errorSentinels[want.Type]; !known && want.Type != "api" {
		t.Fatalf("unknown error type in case: %q", want.Type)
	}
	for typ, sentinel := range errorSentinels {
		if is := errors.Is(err, sentinel); is != (typ == want.Type) {
			t.Errorf("errors.Is(err, sentinel %q) = %v (expected type %q)", typ, is, want.Type)
		}
	}
	if errors.Is(err, ErrInvalidArgument) {
		t.Errorf("an API error must not match ErrInvalidArgument")
	}
	if e.Code != want.Code {
		t.Errorf("Code %q, expected %q", e.Code, want.Code)
	}
	if want.Status != nil && e.StatusCode != *want.Status {
		t.Errorf("StatusCode %d, expected %d", e.StatusCode, *want.Status)
	}
	if want.RequestID != nil {
		expected := *want.RequestID
		if expected == sentRequestID {
			if len(requests) == 0 {
				t.Fatal("$sent without a request")
			}
			expected = requests[len(requests)-1].Header.Get("X-Request-Id")
		}
		if e.RequestID != expected {
			t.Errorf("RequestID %q, expected %q", e.RequestID, expected)
		}
	}
	if want.RetryAfter != nil {
		if expected := time.Duration(*want.RetryAfter * float64(time.Second)); e.RetryAfter != expected {
			t.Errorf("RetryAfter %s, expected %s", e.RetryAfter, expected)
		}
	}
	if want.RequiredScope != nil && e.RequiredScope != *want.RequiredScope {
		t.Errorf("RequiredScope %q, expected %q", e.RequiredScope, *want.RequiredScope)
	}
	if !strings.Contains(err.Error(), want.Code) {
		t.Errorf("the error string must carry the code %q: %q", want.Code, err.Error())
	}
}

// ── JSON comparison ──────────────────────────────────────────────────────────

// isZero reports whether a decoded JSON value is Go's zero-equivalent: null,
// false, 0, "", [] or an object whose values are all zero.
func isZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case bool:
		return !x
	case float64:
		return x == 0
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		for _, e := range x {
			if !isZero(e) {
				return false
			}
		}
		return true
	}
	return false
}

// jsonDiff compares a returned value with expect.result and describes the first
// difference ("" = equal).
//
// Go-specific equivalence (documented, deliberate): the SDK returns typed
// structs, so a key absent on one side is equal to the zero value on the other
// (omitempty drops zero values; a struct field the response lacks marshals as
// its zero value), and an all-zero struct equals null (the pre-existing methods
// return a zero struct — not nil — when the response has no body, and changing
// that would break callers). Every non-zero value must match exactly, which
// the generated sample data guarantees for every field of the spec.
func jsonDiff(path string, got, want any) string {
	if isZero(want) && isZero(got) {
		return ""
	}
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: expected object, got %s", path, mustJSON(got))
		}
		keys := map[string]bool{}
		for k := range w {
			keys[k] = true
		}
		for k := range g {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			if d := jsonDiff(path+"."+k, g[k], w[k]); d != "" {
				return d
			}
		}
		return ""
	case []any:
		g, ok := got.([]any)
		if !ok {
			return fmt.Sprintf("%s: expected list, got %s", path, mustJSON(got))
		}
		if len(g) != len(w) {
			return fmt.Sprintf("%s: %d items, expected %d", path, len(g), len(w))
		}
		for i := range w {
			if d := jsonDiff(fmt.Sprintf("%s[%d]", path, i), g[i], w[i]); d != "" {
				return d
			}
		}
		return ""
	default:
		if string(mustJSON(got)) == string(mustJSON(want)) {
			return ""
		}
		return fmt.Sprintf("%s: %s, expected %s", path, mustJSON(got), mustJSON(want))
	}
}

// bodyDiff compares a sent request body with the expected one: exact, except
// that an expected EMPTY object/list ({} / []) may be absent — Go's omitempty
// cannot tell an explicitly empty map/slice from an unset one, and the SDK
// never sends fields the caller did not fill (BRIEF §3).
func bodyDiff(path string, got, want any) string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: expected object, got %s", path, mustJSON(got))
		}
		for k, wv := range w {
			gv, present := g[k]
			if !present {
				if isEmptyCollection(wv) {
					continue
				}
				return fmt.Sprintf("%s.%s: missing (expected %s)", path, k, mustJSON(wv))
			}
			if d := bodyDiff(path+"."+k, gv, wv); d != "" {
				return d
			}
		}
		for k, gv := range g {
			if _, expected := w[k]; !expected {
				return fmt.Sprintf("%s.%s: not expected (%s)", path, k, mustJSON(gv))
			}
		}
		return ""
	case []any:
		g, ok := got.([]any)
		if !ok {
			return fmt.Sprintf("%s: expected list, got %s", path, mustJSON(got))
		}
		if len(g) != len(w) {
			return fmt.Sprintf("%s: %d items, expected %d", path, len(g), len(w))
		}
		for i := range w {
			if d := bodyDiff(fmt.Sprintf("%s[%d]", path, i), g[i], w[i]); d != "" {
				return d
			}
		}
		return ""
	default:
		if string(mustJSON(got)) == string(mustJSON(want)) {
			return ""
		}
		return fmt.Sprintf("%s: %s, expected %s", path, mustJSON(got), mustJSON(want))
	}
}

func isEmptyCollection(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	}
	return false
}

// ── coverage and signatures ──────────────────────────────────────────────────

// TestConformanceCoverage: every op of cases.json "ops" has a table entry (an
// op without one FAILS — never skipped), no excluded op is mapped, and every
// mapped op exists in "ops".
func TestConformanceCoverage(t *testing.T) {
	file := loadConformance(t)
	if len(file.Ops) == 0 {
		t.Fatal("cases.json has no ops")
	}
	for _, op := range file.Ops {
		if _, mapped := conformanceOps[op]; !mapped {
			t.Errorf("op %q has no method in the Go SDK — implement it and map it in conformanceOps", op)
		}
	}
	withCase := map[string]bool{}
	for _, tc := range file.Cases {
		withCase[tc.Op] = true
	}
	for _, op := range file.Ops {
		if !withCase[op] {
			t.Errorf("op %q has no conformance case", op)
		}
	}
	for _, op := range file.Excluded {
		if _, mapped := conformanceOps[op]; mapped {
			t.Errorf("op %q is in sdk_excluded_ops but mapped in the SDK", op)
		}
	}
	for op := range conformanceOps {
		if !containsString(file.Ops, op) {
			t.Errorf("op %q mapped in the SDK but not in cases.json ops", op)
		}
	}
	t.Logf("%d/%d ops mapped", len(conformanceOps), len(file.Ops))
}

func TestSignatureVectors(t *testing.T) {
	file := loadConformance(t)
	if len(file.Signatures) == 0 {
		t.Fatal("cases.json has no signature vectors")
	}
	for _, v := range file.Signatures {
		if got := VerifyWebhook(v.Secret, []byte(v.Body), v.Signature); got != v.Valid {
			t.Errorf("%s: VerifyWebhook = %v, expected %v", v.ID, got, v.Valid)
		}
		_, err := ConstructWebhookEvent(v.Secret, []byte(v.Body), v.Signature)
		if v.Valid && err != nil {
			t.Errorf("%s: ConstructWebhookEvent: %v", v.ID, err)
		}
		if !v.Valid && !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("%s: ConstructWebhookEvent err = %v, expected ErrInvalidSignature", v.ID, err)
		}
	}
}
