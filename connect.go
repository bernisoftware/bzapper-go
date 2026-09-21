package bzapper

import (
	"context"
	"net/http"
	"net/url"
)

// bZapper Connect (partners): a partner software lets ITS customers subscribe to
// bZapper Pro and connect WhatsApp without leaving the partner's product. The
// partner BACKEND authenticates with the partner secret (bz_partner_...) through
// PartnerClient; once the customer finishes, the partner exchanges the one-time
// code for the customer's API key (bz_live_...) and uses the regular Client.

// Error codes returned by the API to a key issued through Connect.
const (
	// ErrCodeConnectSuspended (HTTP 402): the customer's Pro is unpaid. The key
	// resumes by itself once paid (a connect.resumed webhook is sent).
	ErrCodeConnectSuspended = "connect_suspended"
	// ErrCodeConnectRevoked (HTTP 401): the connection was ended (by the
	// customer, the partner or account deletion). The key never works again.
	ErrCodeConnectRevoked = "connect_revoked"
)

// Lifecycle webhook event types delivered to the partner's webhook. Besides
// these, the partner also receives the regular project events (message.*,
// instance.*, …) of every ACTIVE connection — except QR/pairing codes.
const (
	EventConnectCompleted = "connect.completed"
	EventConnectSuspended = "connect.suspended"
	EventConnectResumed   = "connect.resumed"
	EventConnectRevoked   = "connect.revoked"
)

// ConnectEventTypes lists the Connect lifecycle event types, for reference.
var ConnectEventTypes = []string{
	EventConnectCompleted, EventConnectSuspended, EventConnectResumed, EventConnectRevoked,
}

// ConnectionStatus is the state of a partner connection.
type ConnectionStatus string

const (
	// ConnectionPendingAccount: the customer has no linked bZapper account yet.
	ConnectionPendingAccount ConnectionStatus = "pending_account"
	// ConnectionPendingPayment: account linked, Pro not paid yet.
	ConnectionPendingPayment ConnectionStatus = "pending_payment"
	// ConnectionPendingNumber: Pro paid, WhatsApp number not connected yet.
	ConnectionPendingNumber ConnectionStatus = "pending_number"
	// ConnectionActive: completed — the key works.
	ConnectionActive ConnectionStatus = "active"
	// ConnectionSuspended: the customer's Pro is unpaid — the key answers 402
	// connect_suspended until paid.
	ConnectionSuspended ConnectionStatus = "suspended"
	// ConnectionRevoked: ended.
	ConnectionRevoked ConnectionStatus = "revoked"
)

// ConnectCustomer is the partner's customer, as authenticated in the partner's
// product. Email is required, plus Name or Company.
type ConnectCustomer struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
	// Phone in E.164; pre-fills the WhatsApp number. Optional.
	Phone string `json:"phone,omitempty"`
	// Company becomes the bZapper account and project name. Optional.
	Company string `json:"company,omitempty"`
	// Country is ISO-3166 alpha-2; sets the currency (BR → BRL, Americas → USD,
	// others → EUR). Optional.
	Country string `json:"country,omitempty"`
	Locale  string `json:"locale,omitempty"`
}

// Partner is the partner identity the secret belongs to (GET /partner/me).
type Partner struct {
	ID             string   `json:"id"`
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	LogoURL        string   `json:"logo_url,omitempty"`
	AllowedOrigins []string `json:"allowed_origins,omitempty"`
	WebhookURL     string   `json:"webhook_url,omitempty"`
	KeyScopes      []string `json:"key_scopes,omitempty"`
}

// ConnectionNumber is a WhatsApp number of the connected customer project.
type ConnectionNumber struct {
	ID     string `json:"id"`
	Phone  string `json:"phone"`
	Status string `json:"status"`
}

// PartnerConnection links one partner customer (ExternalID) to one bZapper
// account. Timestamps are RFC3339 strings; the nullable ones are pointers.
type PartnerConnection struct {
	ID         string           `json:"id"`
	PartnerID  string           `json:"partner_id,omitempty"`
	ExternalID string           `json:"external_id"`
	Status     ConnectionStatus `json:"status"`
	// AccountID is the customer's bZapper account (tenant).
	AccountID string             `json:"account_id,omitempty"`
	ProjectID string             `json:"project_id,omitempty"`
	Customer  ConnectCustomer    `json:"customer"`
	Numbers   []ConnectionNumber `json:"numbers,omitempty"`
	// PartnerName and PartnerLogoURL are only filled in ListConnectedApps.
	PartnerName    string  `json:"partner_name,omitempty"`
	PartnerLogoURL string  `json:"partner_logo_url,omitempty"`
	ActivatedAt    *string `json:"activated_at,omitempty"`
	SuspendedAt    *string `json:"suspended_at,omitempty"`
	RevokedAt      *string `json:"revoked_at,omitempty"`
	CreatedAt      string  `json:"created_at,omitempty"`
	UpdatedAt      string  `json:"updated_at,omitempty"`
}

// PartnerConnectionWithKey is a connection plus the raw API key (bz_live_...),
// returned by ExchangeCode and RotateConnectionKey. The key is shown ONLY ONCE —
// store it (use RotateConnectionKey if lost). It is scoped to the customer
// project and cannot touch billing, users, keys or webhooks of the account.
type PartnerConnectionWithKey struct {
	PartnerConnection
	APIKey string `json:"api_key"`
}

// CreateConnectSessionParams is the request body for CreateConnectSession.
type CreateConnectSessionParams struct {
	// ExternalID is YOUR id for this customer (max 200). Same id = same connection.
	ExternalID string          `json:"external_id"`
	Customer   ConnectCustomer `json:"customer"`
	// Locale of the embedded component (e.g. "pt-BR"). Optional.
	Locale string `json:"locale,omitempty"`
}

// ConnectSession is the response of CreateConnectSession. Hand SessionToken to
// your front-end, which opens the embedded component with it.
type ConnectSession struct {
	// SessionToken (cs_...) is short-lived (30 min).
	SessionToken string            `json:"session_token"`
	ExpiresAt    string            `json:"expires_at"`
	Connection   PartnerConnection `json:"connection"`
}

// ListConnectionsParams filters ListConnections. Empty fields are not sent.
type ListConnectionsParams struct {
	ExternalID string
	Status     ConnectionStatus
}

// partnerConnectionList mirrors the {"data": [...]} list envelope.
type partnerConnectionList struct {
	Data []PartnerConnection `json:"data"`
}

// PartnerClient is the bZapper Connect client for a PARTNER backend,
// authenticated with the partner secret (Authorization: Bearer bz_partner_...).
// It shares the transport, options and *Error type with Client. Never ship the
// partner secret to a browser. Safe for concurrent use.
type PartnerClient struct {
	c *Client
}

// NewPartnerClient creates a PartnerClient pointing at the production API. Accepts
// the same options as NewClient (WithBaseURL, WithLocale, WithTimeout,
// WithHTTPClient):
//
//	partner := bzapper.NewPartnerClient(os.Getenv("BZAPPER_PARTNER_SECRET"))
func NewPartnerClient(partnerSecret string, opts ...Option) *PartnerClient {
	return NewPartner(DefaultBaseURL, partnerSecret, opts...)
}

// NewPartner creates a PartnerClient with an explicit base URL ("" = production).
// Prefer NewPartnerClient.
func NewPartner(baseURL, partnerSecret string, opts ...Option) *PartnerClient {
	return &PartnerClient{c: New(baseURL, partnerSecret, opts...)}
}

// Me returns the partner the secret belongs to. GET /partner/me.
func (p *PartnerClient) Me(ctx context.Context) (*Partner, error) {
	var out Partner
	if err := p.c.do(ctx, http.MethodGet, "/partner/me", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateConnectSession creates (or reuses) the connection of your customer and
// returns a short-lived session token that opens the embedded component.
// POST /partner/connect-sessions.
func (p *PartnerClient) CreateConnectSession(ctx context.Context, params CreateConnectSessionParams) (*ConnectSession, error) {
	var out ConnectSession
	if err := p.c.do(ctx, http.MethodPost, "/partner/connect-sessions", nil, params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExchangeCode exchanges the one-time completion code (cc_..., valid 10 min,
// emitted by the component on bzapper:complete) for the customer's API key.
// POST /partner/connect/exchange.
func (p *PartnerClient) ExchangeCode(ctx context.Context, code string) (*PartnerConnectionWithKey, error) {
	var out PartnerConnectionWithKey
	body := map[string]string{"code": code}
	if err := p.c.do(ctx, http.MethodPost, "/partner/connect/exchange", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListConnections lists your connections, optionally filtered by external id
// and/or status. GET /partner/connections.
func (p *PartnerClient) ListConnections(ctx context.Context, filter ListConnectionsParams) ([]PartnerConnection, error) {
	q := url.Values{}
	if filter.ExternalID != "" {
		q.Set("external_id", filter.ExternalID)
	}
	if filter.Status != "" {
		q.Set("status", string(filter.Status))
	}
	var out partnerConnectionList
	if err := p.c.do(ctx, http.MethodGet, "/partner/connections", q, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// GetConnection fetches one connection (status, account, numbers).
// GET /partner/connections/{id}.
func (p *PartnerClient) GetConnection(ctx context.Context, id string) (*PartnerConnection, error) {
	var out PartnerConnection
	if err := p.c.do(ctx, http.MethodGet, "/partner/connections/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RotateConnectionKey issues a new API key for a completed connection; the
// previous key stops working. Returns 409 connection_not_active when the
// connection is not completed yet or was revoked.
// POST /partner/connections/{id}/rotate-key.
func (p *PartnerClient) RotateConnectionKey(ctx context.Context, id string) (*PartnerConnectionWithKey, error) {
	var out PartnerConnectionWithKey
	if err := p.c.do(ctx, http.MethodPost, "/partner/connections/"+url.PathEscape(id)+"/rotate-key", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeConnection ends a connection: revokes the key (it does NOT cancel the
// customer's plan). A connect.revoked webhook is sent.
// DELETE /partner/connections/{id}.
func (p *PartnerClient) RevokeConnection(ctx context.Context, id string) error {
	return p.c.do(ctx, http.MethodDelete, "/partner/connections/"+url.PathEscape(id), nil, nil, nil)
}

// --- Customer side (regular API key) ---

// ListConnectedApps lists the partner apps connected to this account (with
// partner name/logo). GET /me/connections.
func (c *Client) ListConnectedApps(ctx context.Context) ([]PartnerConnection, error) {
	var out partnerConnectionList
	if err := c.do(ctx, http.MethodGet, "/me/connections", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// RevokeConnectedApp disconnects a partner app (admin); the partner's key stops
// working immediately. DELETE /me/connections/{id}.
func (c *Client) RevokeConnectedApp(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/me/connections/"+url.PathEscape(id), nil, nil, nil)
}
