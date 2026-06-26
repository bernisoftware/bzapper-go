package bzapper

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// instanceQuery builds a query carrying instance_id when set.
func instanceQuery(instanceID string) url.Values {
	if instanceID == "" {
		return nil
	}
	return url.Values{"instance_id": {instanceID}}
}

// --- Presence ---

// PresenceChat sets a chat presence indicator (typing, recording, paused). It
// works in groups too: pass a group JID as p.To. InstanceID, To and State are
// sent in the body. POST /presence/chat.
func (c *Client) PresenceChat(ctx context.Context, p PresenceChatParams) error {
	return c.do(ctx, http.MethodPost, "/presence/chat", nil, p, nil)
}

// --- Conversations ---

// ListConversations lists the instance's conversations.
// GET /conversations?instance_id=.
func (c *Client) ListConversations(ctx context.Context, instanceID string) (*ConversationList, error) {
	var out ConversationList
	if err := c.do(ctx, http.MethodGet, "/conversations", instanceQuery(instanceID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ConversationHistory fetches messages of a conversation, newest first. Use
// p.Before (RFC3339) to paginate and p.Limit (≤200) to cap results.
// GET /conversations/{jid}/messages?instance_id=&before=&limit=.
func (c *Client) ConversationHistory(ctx context.Context, jid string, p ConversationHistoryParams) (*ConversationHistoryResult, error) {
	q := instanceQuery(p.InstanceID)
	if q == nil {
		q = url.Values{}
	}
	if p.Before != "" {
		q.Set("before", p.Before)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	var out ConversationHistoryResult
	if err := c.do(ctx, http.MethodGet, "/conversations/"+url.PathEscape(jid)+"/messages", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ArchiveChat archives (on=true) or unarchives (on=false) a chat. InstanceID
// and On are sent in the body. POST /chats/{jid}/archive.
func (c *Client) ArchiveChat(ctx context.Context, jid, instanceID string, on bool) (*ChatFlagResult, error) {
	return c.chatFlag(ctx, jid, "archive", instanceID, on)
}

// PinChat pins (on=true) or unpins (on=false) a chat. InstanceID and On are
// sent in the body. POST /chats/{jid}/pin.
func (c *Client) PinChat(ctx context.Context, jid, instanceID string, on bool) (*ChatFlagResult, error) {
	return c.chatFlag(ctx, jid, "pin", instanceID, on)
}

// MarkChat marks a chat as read (on=true) or unread (on=false). InstanceID and
// On are sent in the body. POST /chats/{jid}/read.
func (c *Client) MarkChat(ctx context.Context, jid, instanceID string, on bool) (*ChatFlagResult, error) {
	return c.chatFlag(ctx, jid, "read", instanceID, on)
}

// chatFlag posts a chat flag toggle (archive, pin, read).
func (c *Client) chatFlag(ctx context.Context, jid, flag, instanceID string, on bool) (*ChatFlagResult, error) {
	body := struct {
		InstanceID string `json:"instance_id"`
		On         bool   `json:"on"`
	}{InstanceID: instanceID, On: on}
	var out ChatFlagResult
	if err := c.do(ctx, http.MethodPost, "/chats/"+url.PathEscape(jid)+"/"+flag, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Groups ---

// ListGroups lists the instance's groups. GET /groups?instance_id=.
func (c *Client) ListGroups(ctx context.Context, instanceID string) (*GroupList, error) {
	var out GroupList
	if err := c.do(ctx, http.MethodGet, "/groups", instanceQuery(instanceID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateGroup creates a group with the given name and participants. instanceID
// goes in the query; p (name, participants) in the body.
// POST /groups?instance_id=.
func (c *Client) CreateGroup(ctx context.Context, instanceID string, p CreateGroupParams) (*Group, error) {
	var out Group
	if err := c.do(ctx, http.MethodPost, "/groups", instanceQuery(instanceID), p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetGroup fetches a single group by JID. GET /groups/{jid}?instance_id=.
func (c *Client) GetGroup(ctx context.Context, jid, instanceID string) (*Group, error) {
	var out Group
	if err := c.do(ctx, http.MethodGet, "/groups/"+url.PathEscape(jid), instanceQuery(instanceID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// JoinGroup joins a group via its invite code. instanceID goes in the query;
// p.Code in the body. POST /groups/join?instance_id=.
func (c *Client) JoinGroup(ctx context.Context, instanceID string, p JoinGroupParams) (*Group, error) {
	var out Group
	if err := c.do(ctx, http.MethodPost, "/groups/join", instanceQuery(instanceID), p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateGroupParticipants adds, removes, promotes or demotes participants.
// instanceID goes in the query; p (action, participants) in the body.
// POST /groups/{jid}/participants?instance_id=.
func (c *Client) UpdateGroupParticipants(ctx context.Context, jid, instanceID string, p UpdateGroupParticipantsParams) (*Group, error) {
	var out Group
	if err := c.do(ctx, http.MethodPost, "/groups/"+url.PathEscape(jid)+"/participants", instanceQuery(instanceID), p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// LeaveGroup leaves a group. POST /groups/{jid}/leave?instance_id=.
func (c *Client) LeaveGroup(ctx context.Context, jid, instanceID string) error {
	return c.do(ctx, http.MethodPost, "/groups/"+url.PathEscape(jid)+"/leave", instanceQuery(instanceID), nil, nil)
}

// GroupInvite returns a group's shareable invite link/code.
// GET /groups/{jid}/invite?instance_id=.
func (c *Client) GroupInvite(ctx context.Context, jid, instanceID string) (*GroupInvite, error) {
	var out GroupInvite
	if err := c.do(ctx, http.MethodGet, "/groups/"+url.PathEscape(jid)+"/invite", instanceQuery(instanceID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Contacts ---

// ContactsCheck checks which phone numbers are registered on WhatsApp.
// InstanceID and Phones are sent in the body. POST /contacts/check.
func (c *Client) ContactsCheck(ctx context.Context, p ContactsCheckParams) (*ContactsCheckResult, error) {
	var out ContactsCheckResult
	if err := c.do(ctx, http.MethodPost, "/contacts/check", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Instance profile ---

// SetProfile updates an instance's WhatsApp profile (display name, status
// message, picture). Unset fields are left unchanged.
// PATCH /instances/{id}/profile.
func (c *Client) SetProfile(ctx context.Context, id string, p SetProfileParams) (*Instance, error) {
	var out Instance
	if err := c.do(ctx, http.MethodPatch, "/instances/"+url.PathEscape(id)+"/profile", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
