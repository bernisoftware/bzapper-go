package bzapper

// This file holds the request and response types used across the SDK. JSON tags
// mirror the bZapper API (openapi.yaml). Optional fields use omitempty and, when
// they need to distinguish "unset" from a zero value, pointer types.

// Role is the role assigned to an API key or user.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleAgent Role = "agent"
)

// InstanceStatus is the connection state of a WhatsApp number.
type InstanceStatus string

const (
	StatusQRPending    InstanceStatus = "qr_pending"
	StatusCodePending  InstanceStatus = "code_pending"
	StatusConnecting   InstanceStatus = "connecting"
	StatusConnected    InstanceStatus = "connected"
	StatusWarming      InstanceStatus = "warming"
	StatusDisconnected InstanceStatus = "disconnected"
	StatusBanned       InstanceStatus = "banned"
)

// ConnectMethod selects how an instance connects.
type ConnectMethod string

const (
	// ConnectQR returns a QR code payload.
	ConnectQR ConnectMethod = "qr"
	// ConnectCode returns an 8-character pairing code.
	ConnectCode ConnectMethod = "code"
)

// SendBase holds the fields common to every message-send request.
type SendBase struct {
	// To is the destination phone in E.164 (e.g. "+5511999999999") or a JID.
	To string `json:"to"`
	// InstanceID pins a specific number, bypassing rotation. Optional.
	InstanceID string `json:"instance_id,omitempty"`
	// PoolID rotates within this pool (when InstanceID is empty). Optional.
	PoolID string `json:"pool_id,omitempty"`
	// QuotedMessageID is the wa_message_id being replied to. Optional.
	QuotedMessageID string `json:"quoted_message_id,omitempty"`
	// QuotedParticipant is the author (phone or JID) of the quoted/reacted
	// message. Only needed in groups when that message is not in the bZapper
	// history (otherwise the author comes from history). Optional.
	QuotedParticipant string `json:"quoted_participant,omitempty"`
	// ClientReference is an end-to-end correlation id echoed back in events.
	ClientReference string `json:"client_reference,omitempty"`
	// Mentions are the people mentioned (group messages): JIDs or plain phones
	// ("5511999999999", "+55 11 99999-9999"). Optional.
	Mentions []string `json:"mentions,omitempty"`
	// Sticky is conversation affinity: with no InstanceID/PoolID, reuse the
	// number already talking to To (support). Defaults to true server-side;
	// set false to force rotation. Optional.
	Sticky *bool `json:"sticky,omitempty"`
	// ScheduledAt schedules the send for a future RFC3339 time. The number is
	// picked at send time. Max lead: Free 24h, Pro 30 days, 1 year with the
	// extended-scheduling add-on. Returns status "scheduled". OTP can't be scheduled.
	ScheduledAt string `json:"scheduled_at,omitempty"`
	// Groups are managed contact-group keys: the send goes to every ACTIVE
	// contact in them (1:1). Optional.
	Groups []string `json:"groups,omitempty"`
	// Tags are contact-tag keys: the send goes to every ACTIVE contact with them
	// (1:1). Optional.
	Tags []string `json:"tags,omitempty"`
	// Force skips the suppression/opt-out guard for this send (use with care —
	// only for transactional messages the contact asked for). Optional.
	Force bool `json:"force,omitempty"`
	// IdempotencyKey is sent as the Idempotency-Key header (not in the body),
	// up to 255 chars. Repeating a send with the same key within 24h (same
	// account) returns the SAME response without sending again — safe retries
	// after timeouts. Same key with a different body → 422
	// idempotency_key_reused; first call still running → 409
	// idempotency_in_progress. Optional.
	IdempotencyKey string `json:"-"`
}

// idempotencyKey exposes SendBase.IdempotencyKey to sendMessage through every
// params struct that embeds SendBase.
func (b SendBase) idempotencyKey() string { return b.IdempotencyKey }

// MediaInput describes media sent by URL or by base64 (use one, never both).
type MediaInput struct {
	URL      string `json:"url,omitempty"`
	Base64   string `json:"base64,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
	Mimetype string `json:"mimetype,omitempty"`
	// PTT, when true on audio, sends a voice note.
	PTT bool `json:"ptt,omitempty"`
}

// Button is a single button in a SendButtons request.
type Button struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title"`
}

// ListRow is a single row in a list section.
type ListRow struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// ListSection is a section of rows in a SendList request.
type ListSection struct {
	Title string    `json:"title,omitempty"`
	Rows  []ListRow `json:"rows"`
}

// --- Message-send request params ---

// SendTextParams is the request for SendText.
type SendTextParams struct {
	SendBase
	// Body is the text content (required).
	Body string `json:"body"`
}

// SendOTPParams is the request for SendOTP.
type SendOTPParams struct {
	SendBase
	// Code is the verification code (required); sent on its own copyable bubble.
	Code string `json:"code"`
	// Body is the optional context text; empty → generated in the account language.
	Body string `json:"body,omitempty"`
	// ExpiryMinutes optionally mentions the expiry in the generated text.
	ExpiryMinutes int `json:"expiry_minutes,omitempty"`
}

// SendMediaParams is the request for image, video, document, audio and sticker
// sends. Set Media.PTT to send an audio message as a voice note.
type SendMediaParams struct {
	SendBase
	Media MediaInput `json:"media"`
}

// SendLocationParams is the request for SendLocation.
type SendLocationParams struct {
	SendBase
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
}

// SendContactParams is the request for SendContact.
type SendContactParams struct {
	SendBase
	ContactName  string `json:"contact_name,omitempty"`
	ContactVCard string `json:"contact_vcard,omitempty"`
}

// SendPollParams is the request for SendPoll.
type SendPollParams struct {
	SendBase
	Name    string   `json:"name"`
	Options []string `json:"options"`
	// SelectableCount is how many options a voter may pick (default 1 server-side).
	SelectableCount int `json:"selectable_count,omitempty"`
}

// SendReactionParams is the request for SendReaction. QuotedMessageID and Emoji
// are required.
type SendReactionParams struct {
	SendBase
	Emoji string `json:"emoji"`
}

// SendButtonsParams is the request for SendButtons. Note: WhatsApp may render
// these as a numbered text menu fallback.
type SendButtonsParams struct {
	SendBase
	Body    string   `json:"body"`
	Footer  string   `json:"footer,omitempty"`
	Buttons []Button `json:"buttons"`
}

// SendListParams is the request for SendList. Note: WhatsApp may render this as
// a numbered text menu fallback.
type SendListParams struct {
	SendBase
	Body       string        `json:"body"`
	Footer     string        `json:"footer,omitempty"`
	ButtonText string        `json:"button_text,omitempty"`
	Sections   []ListSection `json:"sections"`
}

// --- Message-send response ---

// Message is the result of a successful message-send: the queued message
// envelope returned by the API.
type Message struct {
	MessageID       string `json:"message_id"`
	Status          string `json:"status"`
	ClientReference string `json:"client_reference,omitempty"`
	// ScheduledID and ScheduledAt are set when the send was scheduled
	// (Status "scheduled").
	ScheduledID string `json:"scheduled_id,omitempty"`
	ScheduledAt string `json:"scheduled_at,omitempty"`
}

// --- Instances ---

// Instance is a WhatsApp number/instance belonging to the tenant.
type Instance struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id,omitempty"`
	ProjectID    string         `json:"project_id,omitempty"`
	Phone        string         `json:"phone"`
	Nickname     string         `json:"nickname,omitempty"`
	JID          string         `json:"jid,omitempty"`
	Status       InstanceStatus `json:"status"`
	StatusReason string         `json:"status_reason,omitempty"`
	// BannedUntil: quando um ban TEMPORÁRIO expira (o número reconecta sozinho).
	// Vazio em ban permanente (revisão no app + re-pareamento) ou sem ban.
	BannedUntil string `json:"banned_until,omitempty"`
	ProxyURL    string `json:"proxy_url,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`

	// ConsecutiveSendFailures counts sends rejected by WhatsApp in a row. It
	// resets to 0 on the first accepted send. A connected number with a value
	// above zero is alive but not delivering — inspect LastSendErrorCode.
	ConsecutiveSendFailures int `json:"consecutive_send_failures,omitempty"`
	// LastSendErrorCode is the code WhatsApp returned on the last rejected send.
	LastSendErrorCode int    `json:"last_send_error_code,omitempty"`
	LastSendFailureAt string `json:"last_send_failure_at,omitempty"`
	LastSendError     string `json:"last_send_error,omitempty"`

	// WarmingStartedAt is when the number entered the warm-up period.
	WarmingStartedAt string `json:"warming_started_at,omitempty"`
	// HealthScore is the number's health (0–100) used by health_weighted pools.
	HealthScore int `json:"health_score,omitempty"`
	// ArchivedAt is set when the number is archived (see ArchiveInstance).
	ArchivedAt string `json:"archived_at,omitempty"`
}

// ReachOutLockCode is WhatsApp's anti-spam reach-out time-lock. It is a
// per-account limit, not an infrastructure failure: the session stays healthy,
// replies to people who messaged first still go through, and it usually clears
// within hours.
const ReachOutLockCode = 463

// IsReachOutLocked reports whether WhatsApp is refusing this number's sends
// because of the anti-spam reach-out time-lock.
func (i Instance) IsReachOutLocked() bool {
	return i.LastSendErrorCode == ReachOutLockCode && i.ConsecutiveSendFailures > 0
}

// Pagination is the pagination envelope returned with list responses.
type Pagination struct {
	Total  int     `json:"total"`
	Cursor *string `json:"cursor,omitempty"`
}

// InstanceList is the paginated response of ListInstances.
type InstanceList struct {
	Data       []Instance `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// CreateInstanceParams is the request for CreateInstance.
type CreateInstanceParams struct {
	Phone    string `json:"phone"`
	Nickname string `json:"nickname,omitempty"`
	ProxyURL string `json:"proxy_url,omitempty"`
}

// ConnectResult is returned by ConnectInstance.
type ConnectResult struct {
	Status   InstanceStatus `json:"status"`
	QRCode   string         `json:"qr_code,omitempty"`
	PairCode string         `json:"pair_code,omitempty"`
}

// --- API keys ---

// APIKey is an API key's metadata (never the raw key).
type APIKey struct {
	ID         string   `json:"id"`
	TenantID   string   `json:"tenant_id"`
	Name       string   `json:"name,omitempty"`
	Role       Role     `json:"role"`
	Scopes     []string `json:"scopes,omitempty"`
	CreatedAt  string   `json:"created_at,omitempty"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
	RevokedAt  *string  `json:"revoked_at,omitempty"`
	// ProjectID is the project the key belongs to.
	ProjectID string `json:"project_id,omitempty"`
	// PartnerConnectionID is set when the key was issued to a partner via
	// bZapper Connect.
	PartnerConnectionID *string `json:"partner_connection_id,omitempty"`
	// ExpiresAt is when a ROTATED key stops working (the grace period given by
	// RotateKey). Nil while the key was never rotated.
	ExpiresAt *string `json:"expires_at,omitempty"`
	// RotatedTo is the id of the key that replaced this one (set by RotateKey).
	RotatedTo string `json:"rotated_to,omitempty"`
}

// APIKeyList is the response of ListKeys.
type APIKeyList struct {
	Data []APIKey `json:"data"`
}

// CreateKeyParams is the request for CreateKey.
type CreateKeyParams struct {
	Name string `json:"name,omitempty"`
	Role Role   `json:"role,omitempty"`
}

// APIKeyCreated is the response of CreateKey. APIKey is the raw key, shown only
// once and never recoverable — store it securely.
type APIKeyCreated struct {
	APIKey string `json:"api_key"`
	Key    APIKey `json:"key"`
}

// RotateKeyParams is the (optional) request for RotateKey. RevokeInSeconds is
// the grace period kept for the OLD key: nil uses the API default (86400 = 24h),
// a pointer to 0 revokes it immediately, and the maximum is 2592000 (30 days).
type RotateKeyParams struct {
	RevokeInSeconds *int `json:"revoke_in_seconds,omitempty"`
}

// APIKeyRotated is the response of RotateKey. APIKey is the RAW new key, shown
// only once and never recoverable — store it before anything else. PreviousKey
// is the rotated key's metadata and OldKeyExpiresAt is when it stops working
// (nil when it was revoked immediately).
type APIKeyRotated struct {
	APIKey          string  `json:"api_key"`
	Key             APIKey  `json:"key"`
	PreviousKey     *APIKey `json:"previous_key,omitempty"`
	OldKeyExpiresAt *string `json:"old_key_expires_at,omitempty"`
}

// --- Usage ---

// UsageByNumber is the per-number breakdown in a usage summary.
type UsageByNumber struct {
	InstanceID string `json:"instance_id"`
	Phone      string `json:"phone"`
	Total      int    `json:"total"`
}

// UsageSummary is the response of GetUsage.
type UsageSummary struct {
	From         string          `json:"from,omitempty"`
	To           string          `json:"to,omitempty"`
	Total        int             `json:"total"`
	Sent         int             `json:"sent"`
	Received     int             `json:"received"`
	Delivered    int             `json:"delivered"`
	Read         int             `json:"read"`
	Failed       int             `json:"failed"`
	DeliveryRate float64         `json:"delivery_rate"`
	ByType       map[string]int  `json:"by_type,omitempty"`
	ByNumber     []UsageByNumber `json:"by_number,omitempty"`
	// Series is the daily breakdown (sent/received per day).
	Series []UsagePoint `json:"series,omitempty"`
}

// UsagePoint is one day of UsageSummary.Series.
type UsagePoint struct {
	Date     string `json:"date"`
	Sent     int    `json:"sent"`
	Received int    `json:"received"`
}

// GetUsageParams holds the optional date range (RFC3339) for GetUsage.
type GetUsageParams struct {
	From string
	To   string
}

// --- Presence ---

// PresenceState is a chat presence indicator. It also works in groups.
type PresenceState string

const (
	PresenceTyping    PresenceState = "typing"
	PresenceRecording PresenceState = "recording"
	PresencePaused    PresenceState = "paused"
)

// PresenceChatParams is the request for PresenceChat. To may be a contact JID
// or a group JID. InstanceID is required (sent in the body).
type PresenceChatParams struct {
	InstanceID string        `json:"instance_id"`
	To         string        `json:"to"`
	State      PresenceState `json:"state"`
}

// --- Conversations ---

// Conversation is a chat (1:1 or group) the instance participates in.
type Conversation struct {
	// ChatJID, LastType, LastStatus, LastDirection ("in"/"out"), LastBody,
	// LastAt and Unread are the fields the API returns.
	ChatJID       string `json:"chat_jid,omitempty"`
	LastType      string `json:"last_type,omitempty"`
	LastStatus    string `json:"last_status,omitempty"`
	LastDirection string `json:"last_direction,omitempty"`
	LastBody      string `json:"last_body,omitempty"`
	LastAt        string `json:"last_at,omitempty"`
	Unread        int    `json:"unread,omitempty"`

	// Legacy fields (kept for compatibility).
	JID           string `json:"jid"`
	Name          string `json:"name,omitempty"`
	IsGroup       bool   `json:"is_group"`
	UnreadCount   int    `json:"unread_count"`
	Archived      bool   `json:"archived"`
	Pinned        bool   `json:"pinned"`
	LastMessageAt string `json:"last_message_at,omitempty"`
}

// ConversationList is the response of ListConversations.
type ConversationList struct {
	Data       []Conversation `json:"data"`
	Pagination Pagination     `json:"pagination"`
}

// ConversationMessage is a single message in a conversation history.
type ConversationMessage struct {
	ID              string `json:"id,omitempty"`
	InstanceID      string `json:"instance_id,omitempty"`
	Direction       string `json:"direction,omitempty"` // "in" | "out"
	ChatJID         string `json:"chat_jid,omitempty"`
	SenderJID       string `json:"sender_jid,omitempty"`
	SenderLID       string `json:"sender_lid,omitempty"`
	Status          string `json:"status,omitempty"`
	QuotedID        string `json:"quoted_id,omitempty"`
	ClientReference string `json:"client_reference,omitempty"`
	MediaID         string `json:"media_id,omitempty"`
	// Payload is the message content. A pointer (not a map) keeps
	// ConversationMessage comparable, as it was before this field existed.
	Payload   *map[string]any `json:"payload,omitempty"`
	CreatedAt string          `json:"created_at,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`

	WAMessageID string `json:"wa_message_id"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	FromMe      bool   `json:"from_me"`
	Type        string `json:"type,omitempty"`
	Body        string `json:"body,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
}

// ConversationHistoryParams is the query for ConversationHistory. Before is an
// RFC3339 timestamp for pagination; Limit must be ≤ 200. InstanceID is required.
type ConversationHistoryParams struct {
	InstanceID string
	Before     string
	Limit      int
}

// ConversationHistory is the response of ConversationHistory.
type ConversationHistoryResult struct {
	Data       []ConversationMessage `json:"data"`
	Pagination Pagination            `json:"pagination"`
}

// ChatFlagResult is the response of the chat flag toggles (archive, pin, read).
type ChatFlagResult struct {
	JID string `json:"jid"`
	On  bool   `json:"on"`
}

// --- Groups ---

// GroupParticipant is a member of a group.
type GroupParticipant struct {
	JID string `json:"jid"`
	// Phone (+DDIdigits) and LID of the member, when known: in LID-addressed
	// groups JID is the @lid and only Phone identifies the person.
	Phone        string `json:"phone,omitempty"`
	LID          string `json:"lid,omitempty"`
	IsAdmin      bool   `json:"is_admin"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

// Group is a WhatsApp group.
type Group struct {
	JID          string             `json:"jid"`
	Name         string             `json:"name,omitempty"`
	Topic        string             `json:"topic,omitempty"`
	Owner        string             `json:"owner,omitempty"`
	Size         int                `json:"size,omitempty"`     // participant count
	Announce     bool               `json:"announce,omitempty"` // only admins send
	Locked       bool               `json:"locked,omitempty"`   // only admins edit info
	Participants []GroupParticipant `json:"participants,omitempty"`
	CreatedAt    string             `json:"created_at,omitempty"`
}

// GroupList is the response of ListGroups.
type GroupList struct {
	Data       []Group    `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// CreateGroupParams is the request body for CreateGroup. InstanceID is sent in
// the query (set on the method call), Name and Participants in the body.
type CreateGroupParams struct {
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

// JoinGroupParams is the request body for JoinGroup. Code is the invite code.
type JoinGroupParams struct {
	Code string `json:"code"`
}

// GroupParticipantAction is the action applied by UpdateGroupParticipants.
type GroupParticipantAction string

const (
	GroupAdd     GroupParticipantAction = "add"
	GroupRemove  GroupParticipantAction = "remove"
	GroupPromote GroupParticipantAction = "promote"
	GroupDemote  GroupParticipantAction = "demote"
)

// UpdateGroupParticipantsParams is the request body for UpdateGroupParticipants.
type UpdateGroupParticipantsParams struct {
	Action       GroupParticipantAction `json:"action"`
	Participants []string               `json:"participants"`
}

// GroupInvite is the response of GroupInvite — a shareable invite link/code.
type GroupInvite struct {
	// InviteLink is the shareable link (https://chat.whatsapp.com/...), as
	// returned by the API.
	InviteLink string `json:"invite_link,omitempty"`
	Code       string `json:"code"`
	URL        string `json:"url,omitempty"`
}

// --- Contacts ---

// ContactsCheckParams is the request for ContactsCheck. InstanceID and Phones
// are sent in the body. Phones are in E.164.
type ContactsCheckParams struct {
	InstanceID string   `json:"instance_id"`
	Phones     []string `json:"phones"`
}

// ContactCheck is a single phone's WhatsApp registration result.
type ContactCheck struct {
	// Query is the phone as sent; InWhatsApp tells whether it has WhatsApp;
	// JID/LID identify the account when it does.
	Query      string `json:"query,omitempty"`
	InWhatsApp bool   `json:"in_whatsapp,omitempty"`
	LID        string `json:"lid,omitempty"`

	Phone        string `json:"phone"`
	IsRegistered bool   `json:"is_registered"`
	JID          string `json:"jid,omitempty"`
}

// ContactsCheckResult is the response of ContactsCheck.
type ContactsCheckResult struct {
	Data []ContactCheck `json:"data"`
}

// --- Instance profile ---

// SetProfileParams is the request for SetProfile. All fields are optional;
// pointer types distinguish "unset" from an explicit empty value. Picture is a
// base64-encoded image or URL, depending on the server.
type SetProfileParams struct {
	DisplayName   *string `json:"display_name,omitempty"`
	StatusMessage *string `json:"status_message,omitempty"`
	Picture       *string `json:"picture,omitempty"`
}

// --- Account contacts (captured from conversations, shared across the account) ---

// ContactRecord is a contact captured automatically from incoming conversations.
type ContactRecord struct {
	ID            string `json:"id"`
	ChatJID       string `json:"chat_jid"`
	Phone         string `json:"phone"`
	Name          string `json:"name"`
	AvatarURL     string `json:"avatar_url"`
	InstanceID    string `json:"instance_id,omitempty"`
	MessageCount  int    `json:"message_count"`
	LastMessageAt string `json:"last_message_at,omitempty"`

	Email        string          `json:"email,omitempty"`
	Document     string          `json:"document,omitempty"`
	DocumentType string          `json:"document_type,omitempty"`
	Address      *ContactAddress `json:"address,omitempty"`
	// Status: active, pending_validation, opted_out, blocked or unreachable.
	Status       string `json:"status,omitempty"`
	StatusReason string `json:"status_reason,omitempty"`
	// Source: inbound, outbound, api, import or widget.
	Source     string `json:"source,omitempty"`
	OptedOutAt string `json:"opted_out_at,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	// Tags and Groups are the contact's tag / contact-group keys. Pointers (not
	// slices) keep ContactRecord comparable, as it was before these fields.
	Tags   *[]string `json:"tags,omitempty"`
	Groups *[]string `json:"groups,omitempty"`
}

// ContactRecordList is the response of ListContacts.
type ContactRecordList struct {
	Data   []ContactRecord `json:"data"`
	Total  int             `json:"total,omitempty"`
	Limit  int             `json:"limit,omitempty"`
	Offset int             `json:"offset,omitempty"`
}

// ListContactsParams is the query for ListContacts. ProjectID filters by
// project: a project id or "current" (the one bound to your key). InstanceID
// filters by a number (instance) the contact interacted with — the contact↔number
// link is maintained automatically by the API. All optional.
type ListContactsParams struct {
	Search     string
	ProjectID  string
	InstanceID string
	Limit      int

	// Tags / Groups filter by tag / contact-group keys, comma-separated
	// ("vip,lead" — the wire format; a string keeps ListContactsParams
	// comparable). TagsMatch is "any" (default) or "all".
	Tags      string
	TagsMatch string
	Groups    string
	// Status: active, pending_validation, opted_out, blocked or unreachable.
	Status   string
	City     string
	State    string
	Country  string
	Zip      string
	Document string
	// HasEmail: only contacts that have (true) or lack (false) an email.
	HasEmail *bool
	// Date filters, ISO 8601 / RFC 3339 (e.g. "2026-09-01T00:00:00Z").
	LastActivityAfter  string
	LastActivityBefore string
	CreatedAfter       string
	CreatedBefore      string
	// Sort order (e.g. "last_activity").
	Sort   string
	Offset int
}

// ListInstancesParams is the optional query for ListInstances. ProjectID scopes
// the numbers by project: a project id, or "all" for every number in the account.
// Empty uses the active project (X-Project-Id). Optional.
type ListInstancesParams struct {
	ProjectID string
	// Archived lists archived numbers ("1"/"true") instead of the active ones.
	Archived string
}

// --- Projects (numbers, inbox, keys and stats are isolated per project) ---

// Project is an isolated environment (numbers, inbox, keys, stats) in the account.
type Project struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	Name      string `json:"name"`
	LogoURL   string `json:"logo_url"`
	Color     string `json:"color"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	// APIMode is UNOFFICIAL (WhatsApp Web) or OFFICIAL (WhatsApp Cloud API);
	// fixed at creation.
	APIMode    string `json:"api_mode,omitempty"`
	ArchivedAt string `json:"archived_at,omitempty"`
}

// ProjectList is the response of ListProjects.
type ProjectList struct {
	Data []Project `json:"data"`
}

// --- Brand (number identity kit + "About"; lives in the project) ---

// BrandProfile is the numbers' identity (brand kit + "About").
type BrandProfile struct {
	About       string `json:"about,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	LogoURL     string `json:"logo_url,omitempty"`
	Website     string `json:"website,omitempty"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	Address     string `json:"address,omitempty"`
	Description string `json:"description,omitempty"`
}

// BrandApplyResult is the response of ApplyBrand.
type BrandApplyResult struct {
	Applied int      `json:"applied"`
	Skipped []string `json:"skipped"`
	Total   int      `json:"total"`
}

// --- Account users (admin) ---

// AccountUser is a user in the account. Role admin (everything) or agent
// (member — no billing).
type AccountUser struct {
	ID              string  `json:"id"`
	Email           string  `json:"email"`
	Name            string  `json:"name"`
	Role            Role    `json:"role"`
	AvatarURL       string  `json:"avatar_url,omitempty"`
	EmailVerifiedAt *string `json:"email_verified_at,omitempty"`
}

// AccountUserList is the response of ListUsers.
type AccountUserList struct {
	Data []AccountUser `json:"data"`
}

// InviteUserParams is the request for InviteUser. Role is "admin" or "agent".
type InviteUserParams struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	Role  Role   `json:"role,omitempty"`
}

// --- Account usage (aggregate + per project; admin) ---

// ProjectUsage is the per-project breakdown in an account usage report.
type ProjectUsage struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Numbers   int    `json:"numbers"`
	Total     int    `json:"total"`
	Sent      int    `json:"sent"`
	Received  int    `json:"received"`
}

// AccountUsage is the account's aggregate usage plus a per-project breakdown.
type AccountUsage struct {
	Account  UsageSummary   `json:"account"`
	Projects []ProjectUsage `json:"projects"`
}
