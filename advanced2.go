package bzapper

import (
	"context"
	"net/http"
)

// --- Messages (edit/revoke/forward/read), chats, labels, block, calls,
// groups and instance management added in the r2 SDK standard. ---

// MessageRef identifies a message produced by EditMessage / ForwardMessage.
type MessageRef struct {
	WAMessageID string `json:"wa_message_id"`
}

// ForwardMessageParams is the request for ForwardMessage.
type ForwardMessageParams struct {
	InstanceID  string `json:"instance_id"`
	To          string `json:"to"`
	FromChat    string `json:"from_chat"`
	WAMessageID string `json:"wa_message_id"`
}

// MarkReadParams is the request for MarkRead.
type MarkReadParams struct {
	InstanceID   string   `json:"instance_id"`
	Chat         string   `json:"chat"`
	WAMessageIDs []string `json:"wa_message_ids,omitempty"`
	Sender       string   `json:"sender,omitempty"`
}

// PrivacyParams is the request for SetPrivacy (e.g. Setting "last", Value
// "contacts").
type PrivacyParams struct {
	Setting string `json:"setting"`
	Value   string `json:"value"`
}

// ApplyChatLabelParams is the request for ApplyChatLabel (Apply=false removes).
type ApplyChatLabelParams struct {
	InstanceID string `json:"instance_id"`
	LabelID    string `json:"label_id"`
	Apply      bool   `json:"apply"`
}

// Label is a WhatsApp Business chat label.
type Label struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

// LabelList is the response of ListLabels.
type LabelList struct {
	Data []Label `json:"data"`
}

// CreateLabelParams is the request for CreateLabel.
type CreateLabelParams struct {
	InstanceID string `json:"instance_id"`
	Name       string `json:"name"`
	Color      string `json:"color,omitempty"`
}

// Blocklist is the response of GetBlocklist (blocked JIDs).
type Blocklist struct {
	Data []string `json:"data"`
}

// RejectCallParams is the request for RejectCall.
type RejectCallParams struct {
	InstanceID string `json:"instance_id"`
	CallFrom   string `json:"call_from"`
	CallID     string `json:"call_id"`
}

// OfferCallParams is the request for OfferCall.
type OfferCallParams struct {
	InstanceID string `json:"instance_id"`
	To         string `json:"to"`
	Video      bool   `json:"video,omitempty"`
}

// CallOffer is the response of OfferCall.
type CallOffer struct {
	CallID string `json:"call_id"`
}

// UpdateGroupParams is the request for UpdateGroup. Nil fields are not sent.
type UpdateGroupParams struct {
	Name     *string `json:"name,omitempty"`
	Topic    *string `json:"topic,omitempty"`
	Announce *bool   `json:"announce,omitempty"` // only admins send
	Locked   *bool   `json:"locked,omitempty"`   // only admins edit info
}

// GroupInviteLinkParams is the query for GroupInviteLink. Reset revokes the
// current link and issues a new one.
type GroupInviteLinkParams struct {
	InstanceID string
	Reset      bool
}

// JoinRequest is a pending request to join a group.
type JoinRequest struct {
	JID string `json:"jid"`
}

// JoinRequestList is the response of ListJoinRequests.
type JoinRequestList struct {
	Data []JoinRequest `json:"data"`
}

// UpdateJoinRequestsParams approves (Approve=true) or rejects pending requests.
type UpdateJoinRequestsParams struct {
	Participants []string `json:"participants"`
	Approve      bool     `json:"approve"`
}

// InboundFilters controls which inbound messages a number ignores. In
// SetInboundFilters, nil fields are not sent (left unchanged).
type InboundFilters struct {
	IgnoreBroadcast *bool    `json:"ignore_broadcast,omitempty"`
	IgnoreStatus    *bool    `json:"ignore_status,omitempty"`
	IgnoreGroups    *bool    `json:"ignore_groups,omitempty"`
	GroupAllowlist  []string `json:"group_allowlist,omitempty"`
	GroupDenylist   []string `json:"group_denylist,omitempty"`
}

// EditMessage edits a sent text message. PATCH /messages/{id}.
func (c *Client) EditMessage(ctx context.Context, id, text string) (*MessageRef, error) {
	return call[MessageRef](ctx, c, apiRequest{method: http.MethodPatch, path: "/messages/" + seg(id),
		body: struct {
			Text string `json:"text"`
		}{text}})
}

// RevokeMessage deletes a sent message (forEveryone=true deletes for everyone).
// DELETE /messages/{id}.
func (c *Client) RevokeMessage(ctx context.Context, id string, forEveryone bool) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/messages/" + seg(id),
		query: newQuery().boolTrue("for_everyone", forEveryone).values()})
}

// ForwardMessage forwards a message to another chat. POST /messages/forward.
func (c *Client) ForwardMessage(ctx context.Context, p ForwardMessageParams) (*MessageRef, error) {
	return call[MessageRef](ctx, c, apiRequest{method: http.MethodPost, path: "/messages/forward", body: p})
}

// MarkRead sends read receipts for messages of a chat. POST /messages/{id}/read.
func (c *Client) MarkRead(ctx context.Context, id string, p MarkReadParams) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/messages/" + seg(id) + "/read", body: p})
}

// SetPrivacy changes a WhatsApp privacy setting of the number.
// PATCH /instances/{id}/privacy.
func (c *Client) SetPrivacy(ctx context.Context, id string, p PrivacyParams) error {
	return c.exec(ctx, apiRequest{method: http.MethodPatch, path: "/instances/" + seg(id) + "/privacy", body: p})
}

// MuteChat mutes (on=true) or unmutes a chat. POST /chats/{jid}/mute.
func (c *Client) MuteChat(ctx context.Context, jid, instanceID string, on bool) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/chats/" + seg(jid) + "/mute",
		body: struct {
			InstanceID string `json:"instance_id"`
			On         bool   `json:"on"`
		}{instanceID, on}})
}

// ApplyChatLabel applies (or removes) a label on a chat. POST /chats/{jid}/labels.
func (c *Client) ApplyChatLabel(ctx context.Context, jid string, p ApplyChatLabelParams) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/chats/" + seg(jid) + "/labels", body: p})
}

// ListLabels lists the number's labels. GET /labels?instance_id=.
func (c *Client) ListLabels(ctx context.Context, instanceID string) (*LabelList, error) {
	return call[LabelList](ctx, c, apiRequest{method: http.MethodGet, path: "/labels", query: instanceQuery(instanceID)})
}

// CreateLabel creates a label. POST /labels.
func (c *Client) CreateLabel(ctx context.Context, p CreateLabelParams) (*Label, error) {
	return call[Label](ctx, c, apiRequest{method: http.MethodPost, path: "/labels", body: p})
}

// DeleteLabel deletes a label. DELETE /labels/{id}?instance_id=.
func (c *Client) DeleteLabel(ctx context.Context, id, instanceID string) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/labels/" + seg(id), query: instanceQuery(instanceID)})
}

// BlockContact blocks a contact on the number. POST /contacts/{jid}/block.
func (c *Client) BlockContact(ctx context.Context, jid, instanceID string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/contacts/" + seg(jid) + "/block", body: instanceBody(instanceID)})
}

// UnblockContact unblocks a contact. POST /contacts/{jid}/unblock.
func (c *Client) UnblockContact(ctx context.Context, jid, instanceID string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/contacts/" + seg(jid) + "/unblock", body: instanceBody(instanceID)})
}

// GetBlocklist lists the JIDs blocked by the number. GET /blocklist?instance_id=.
func (c *Client) GetBlocklist(ctx context.Context, instanceID string) (*Blocklist, error) {
	return call[Blocklist](ctx, c, apiRequest{method: http.MethodGet, path: "/blocklist", query: instanceQuery(instanceID)})
}

// RejectCall rejects an incoming call. POST /calls/reject.
func (c *Client) RejectCall(ctx context.Context, p RejectCallParams) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/calls/reject", body: p})
}

// OfferCall places a call (rings the contact). POST /calls/offer.
func (c *Client) OfferCall(ctx context.Context, p OfferCallParams) (*CallOffer, error) {
	return call[CallOffer](ctx, c, apiRequest{method: http.MethodPost, path: "/calls/offer", body: p})
}

// UpdateGroup changes a group's name, topic and settings.
// PATCH /groups/{jid}?instance_id=.
func (c *Client) UpdateGroup(ctx context.Context, jid, instanceID string, p UpdateGroupParams) error {
	return c.exec(ctx, apiRequest{method: http.MethodPatch, path: "/groups/" + seg(jid), query: instanceQuery(instanceID), body: p})
}

// GroupInviteLink returns (or, with Reset, regenerates) a group's invite link.
// GET /groups/{jid}/invite?instance_id=&reset=.
func (c *Client) GroupInviteLink(ctx context.Context, jid string, p GroupInviteLinkParams) (*GroupInvite, error) {
	return call[GroupInvite](ctx, c, apiRequest{method: http.MethodGet, path: "/groups/" + seg(jid) + "/invite",
		query: newQuery().str("instance_id", p.InstanceID).boolTrue("reset", p.Reset).values()})
}

// ListJoinRequests lists pending join requests of a group.
// GET /groups/{jid}/join-requests?instance_id=.
func (c *Client) ListJoinRequests(ctx context.Context, jid, instanceID string) (*JoinRequestList, error) {
	return call[JoinRequestList](ctx, c, apiRequest{method: http.MethodGet, path: "/groups/" + seg(jid) + "/join-requests", query: instanceQuery(instanceID)})
}

// UpdateJoinRequests approves or rejects pending join requests.
// POST /groups/{jid}/join-requests?instance_id=.
func (c *Client) UpdateJoinRequests(ctx context.Context, jid, instanceID string, p UpdateJoinRequestsParams) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/groups/" + seg(jid) + "/join-requests", query: instanceQuery(instanceID), body: p})
}

// SetInboundFilters changes which inbound messages the number ignores.
// PATCH /instances/{id}/inbound-filters.
func (c *Client) SetInboundFilters(ctx context.Context, id string, p InboundFilters) (*InboundFilters, error) {
	return call[InboundFilters](ctx, c, apiRequest{method: http.MethodPatch, path: "/instances/" + seg(id) + "/inbound-filters", body: p})
}

// SetInstanceProxy sets the number's outbound proxy ("" removes it).
// PATCH /instances/{id}/proxy.
func (c *Client) SetInstanceProxy(ctx context.Context, id, proxyURL string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPatch, path: "/instances/" + seg(id) + "/proxy",
		body: struct {
			ProxyURL string `json:"proxy_url"`
		}{proxyURL}})
}

// DeleteInstance deletes a number. DELETE /instances/{id}.
func (c *Client) DeleteInstance(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/instances/" + seg(id)})
}

// LogoutInstance logs the number out of WhatsApp (pair again to reconnect).
// POST /instances/{id}/logout.
func (c *Client) LogoutInstance(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/instances/" + seg(id) + "/logout"})
}

// ArchiveInstance archives a number (hidden from the default listing).
// POST /instances/{id}/archive.
func (c *Client) ArchiveInstance(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/instances/" + seg(id) + "/archive"})
}

// UnarchiveInstance restores an archived number. POST /instances/{id}/unarchive.
func (c *Client) UnarchiveInstance(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/instances/" + seg(id) + "/unarchive"})
}

func instanceBody(instanceID string) any {
	return struct {
		InstanceID string `json:"instance_id"`
	}{instanceID}
}
