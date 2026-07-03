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
	// ClientReference is an end-to-end correlation id echoed back in events.
	ClientReference string `json:"client_reference,omitempty"`
	// Mentions are JIDs mentioned (group messages). Optional.
	Mentions []string `json:"mentions,omitempty"`
	// Sticky is conversation affinity: with no InstanceID/PoolID, reuse the
	// number already talking to To (support). Defaults to true server-side;
	// set false to force rotation. Optional.
	Sticky *bool `json:"sticky,omitempty"`
	// ScheduledAt schedules the send for a future RFC3339 time. The number is
	// picked at send time. Max lead: Free 24h, Pro 30 days, 1 year with the
	// extended-scheduling add-on. Returns status "scheduled". OTP can't be scheduled.
	ScheduledAt string `json:"scheduled_at,omitempty"`
}

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
}

// --- Instances ---

// Instance is a WhatsApp number/instance belonging to the tenant.
type Instance struct {
	ID           string         `json:"id"`
	Phone        string         `json:"phone"`
	Nickname     string         `json:"nickname,omitempty"`
	JID          string         `json:"jid,omitempty"`
	Status       InstanceStatus `json:"status"`
	StatusReason string         `json:"status_reason,omitempty"`
	ProxyURL     string         `json:"proxy_url,omitempty"`
	CreatedAt    string         `json:"created_at,omitempty"`
	UpdatedAt    string         `json:"updated_at,omitempty"`
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
	JID          string `json:"jid"`
	IsAdmin      bool   `json:"is_admin"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

// Group is a WhatsApp group.
type Group struct {
	JID          string             `json:"jid"`
	Name         string             `json:"name,omitempty"`
	Topic        string             `json:"topic,omitempty"`
	Owner        string             `json:"owner,omitempty"`
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
	Code string `json:"code"`
	URL  string `json:"url,omitempty"`
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
}

// ContactRecordList is the response of ListContacts.
type ContactRecordList struct {
	Data []ContactRecord `json:"data"`
}

// ListContactsParams is the query for ListContacts. ProjectID filters by
// project: a project id or "current" (the one bound to your key). All optional.
type ListContactsParams struct {
	Search    string
	ProjectID string
	Limit     int
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
