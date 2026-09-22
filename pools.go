package bzapper

import (
	"context"
	"net/http"
)

// --- Pools, official (Cloud API) account, campaigns and webhook extras ---

// PoolStrategy is how a pool rotates its numbers.
type PoolStrategy string

const (
	PoolRoundRobin     PoolStrategy = "round_robin"
	PoolLeastUsed      PoolStrategy = "least_used"
	PoolHealthWeighted PoolStrategy = "health_weighted"
)

// Pool is a group of numbers used in rotation.
type Pool struct {
	ID        string       `json:"id"`
	TenantID  string       `json:"tenant_id,omitempty"`
	Name      string       `json:"name,omitempty"`
	Strategy  PoolStrategy `json:"strategy,omitempty"`
	IsDefault bool         `json:"is_default,omitempty"`
	Members   []string     `json:"members,omitempty"` // instance ids
	CreatedAt string       `json:"created_at,omitempty"`
	UpdatedAt string       `json:"updated_at,omitempty"`
}

// PoolList is the response of ListPools.
type PoolList struct {
	Data []Pool `json:"data"`
}

// CreatePoolParams is the request for CreatePool.
type CreatePoolParams struct {
	Name      string       `json:"name,omitempty"`
	Strategy  PoolStrategy `json:"strategy,omitempty"`
	IsDefault bool         `json:"is_default,omitempty"`
}

// OfficialAccount is the WhatsApp Cloud API (official rail) account of the
// project.
type OfficialAccount struct {
	ID             string `json:"id,omitempty"`
	TenantID       string `json:"tenant_id,omitempty"`
	ProjectID      string `json:"project_id,omitempty"`
	WabaID         string `json:"waba_id,omitempty"`
	PhoneNumberID  string `json:"phone_number_id,omitempty"`
	DisplayNumber  string `json:"display_number,omitempty"`
	VerifiedName   string `json:"verified_name,omitempty"`
	Status         string `json:"status,omitempty"` // PENDENTE, AGUARDANDO_PAGAMENTO, ATIVA, SUSPENSA
	StatusReason   string `json:"status_reason,omitempty"`
	QualityRating  string `json:"quality_rating,omitempty"`
	MessagingLimit string `json:"messaging_limit,omitempty"`
	Source         string `json:"source,omitempty"` // embedded_signup | manual
}

// ConnectOfficialAccountParams is the request for ConnectOfficialAccount.
type ConnectOfficialAccountParams struct {
	WabaID        string `json:"waba_id"`
	PhoneNumberID string `json:"phone_number_id"`
	AccessToken   string `json:"access_token"`
	DisplayNumber string `json:"display_number,omitempty"`
	VerifiedName  string `json:"verified_name,omitempty"`
	Status        string `json:"status,omitempty"`
}

// NumberEligibility tells whether a number can dispatch campaigns.
type NumberEligibility struct {
	InstanceID    string `json:"instance_id,omitempty"`
	Phone         string `json:"phone,omitempty"`
	Nickname      string `json:"nickname,omitempty"`
	Status        string `json:"status,omitempty"`
	ConnectedDays int    `json:"connected_days,omitempty"`
	Health        int    `json:"health,omitempty"`
	Eligible      bool   `json:"eligible,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// CampaignEligibility is the response of GetCampaignEligibility.
type CampaignEligibility struct {
	WarmupDays    int                 `json:"warmup_days,omitempty"`
	MinNumbers    int                 `json:"min_numbers,omitempty"`
	EligibleCount int                 `json:"eligible_count,omitempty"`
	CanDispatch   bool                `json:"can_dispatch,omitempty"`
	Reason        string              `json:"reason,omitempty"`
	Numbers       []NumberEligibility `json:"numbers,omitempty"`
}

// CampaignEligibilityParams is the optional query of GetCampaignEligibility.
type CampaignEligibilityParams struct {
	PoolID string
}

// CampaignMedia is the response of UploadCampaignMedia (use URL in a
// variation's media).
type CampaignMedia struct {
	URL string `json:"url"`
}

// CampaignStatusChange is the response of Pause/Resume/CancelCampaignWithResult.
// Status is "paused", "running" or "canceled".
type CampaignStatusChange struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// CampaignStartResult is the response of StartCampaignWithResult.
type CampaignStartResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	StartAt string `json:"start_at,omitempty"`
	// Waiting/StartsAt are set while the send window (08h–21h São Paulo) holds
	// the campaign.
	Waiting  string `json:"waiting,omitempty"`
	StartsAt string `json:"starts_at,omitempty"`
}

// ListCampaignsParams is the optional query of ListCampaignsWithParams.
type ListCampaignsParams struct {
	Limit int
}

// ListScheduledParams is the optional query of ListScheduledWithParams.
type ListScheduledParams struct {
	Limit int
}

// ListCampaignRecipientsParams is the optional query of
// ListCampaignRecipientsWithParams.
type ListCampaignRecipientsParams struct {
	Limit int
}

// WebhookEventRef is the response of TriggerWebhookEvent.
type WebhookEventRef struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
}

// ListPools lists the project's pools. GET /pools.
func (c *Client) ListPools(ctx context.Context) (*PoolList, error) {
	return call[PoolList](ctx, c, apiRequest{method: http.MethodGet, path: "/pools"})
}

// CreatePool creates a pool. POST /pools.
func (c *Client) CreatePool(ctx context.Context, p CreatePoolParams) (*Pool, error) {
	return call[Pool](ctx, c, apiRequest{method: http.MethodPost, path: "/pools", body: p})
}

// GetPool fetches a pool. GET /pools/{id}.
func (c *Client) GetPool(ctx context.Context, id string) (*Pool, error) {
	return call[Pool](ctx, c, apiRequest{method: http.MethodGet, path: "/pools/" + seg(id)})
}

// AddPoolNumber adds a number to a pool. POST /pools/{id}/numbers.
func (c *Client) AddPoolNumber(ctx context.Context, id, instanceID string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/pools/" + seg(id) + "/numbers", body: instanceBody(instanceID)})
}

// GetOfficialAccount returns the project's Cloud API account. GET /official/account.
func (c *Client) GetOfficialAccount(ctx context.Context) (*OfficialAccount, error) {
	return call[OfficialAccount](ctx, c, apiRequest{method: http.MethodGet, path: "/official/account"})
}

// ConnectOfficialAccount links a Cloud API account (manual onboarding).
// POST /official/account.
func (c *Client) ConnectOfficialAccount(ctx context.Context, p ConnectOfficialAccountParams) (*OfficialAccount, error) {
	return call[OfficialAccount](ctx, c, apiRequest{method: http.MethodPost, path: "/official/account", body: p})
}

// DisconnectOfficialAccount unlinks the Cloud API account. DELETE /official/account.
func (c *Client) DisconnectOfficialAccount(ctx context.Context) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/official/account"})
}

// GetCampaignEligibility tells which numbers can dispatch campaigns (warm-up,
// health). GET /campaigns/eligibility.
func (c *Client) GetCampaignEligibility(ctx context.Context, p CampaignEligibilityParams) (*CampaignEligibility, error) {
	return call[CampaignEligibility](ctx, c, apiRequest{method: http.MethodGet, path: "/campaigns/eligibility",
		query: newQuery().str("pool_id", p.PoolID).values()})
}

// UploadCampaignMedia uploads a media file for campaign variations (multipart).
// POST /campaigns/media.
func (c *Client) UploadCampaignMedia(ctx context.Context, file UploadFile) (*CampaignMedia, error) {
	r, err := multipartRequest(http.MethodPost, "/campaigns/media", file, nil)
	if err != nil {
		return nil, err
	}
	return call[CampaignMedia](ctx, c, r)
}

// PauseCampaignWithResult pauses the campaign and returns its new state —
// PauseCampaign(ctx, id) remains and discards it. POST /campaigns/{id}/pause.
func (c *Client) PauseCampaignWithResult(ctx context.Context, id string) (*CampaignStatusChange, error) {
	return call[CampaignStatusChange](ctx, c, apiRequest{method: http.MethodPost, path: "/campaigns/" + seg(id) + "/pause"})
}

// ResumeCampaignWithResult resumes the campaign and returns its new state —
// ResumeCampaign(ctx, id) remains and discards it. POST /campaigns/{id}/resume.
func (c *Client) ResumeCampaignWithResult(ctx context.Context, id string) (*CampaignStatusChange, error) {
	return call[CampaignStatusChange](ctx, c, apiRequest{method: http.MethodPost, path: "/campaigns/" + seg(id) + "/resume"})
}

// CancelCampaignWithResult cancels the campaign and returns its new state —
// CancelCampaign(ctx, id) remains and discards it. POST /campaigns/{id}/cancel.
func (c *Client) CancelCampaignWithResult(ctx context.Context, id string) (*CampaignStatusChange, error) {
	return call[CampaignStatusChange](ctx, c, apiRequest{method: http.MethodPost, path: "/campaigns/" + seg(id) + "/cancel"})
}

// StartCampaignWithResult starts (or schedules) the campaign and returns its
// new state — StartCampaign(ctx, id) remains and discards it.
// POST /campaigns/{id}/start.
func (c *Client) StartCampaignWithResult(ctx context.Context, id string) (*CampaignStartResult, error) {
	return call[CampaignStartResult](ctx, c, apiRequest{method: http.MethodPost, path: "/campaigns/" + seg(id) + "/start"})
}

// ListCampaignsWithParams lists the project's campaigns with a limit.
// GET /campaigns?limit=.
func (c *Client) ListCampaignsWithParams(ctx context.Context, p ListCampaignsParams) ([]Campaign, error) {
	var out struct {
		Data []Campaign `json:"data"`
	}
	err := c.do(ctx, http.MethodGet, "/campaigns", newQuery().int("limit", p.Limit).values(), nil, &out)
	return out.Data, err
}

// ListScheduledWithParams lists scheduled sends with a limit.
// GET /messages/scheduled?limit=.
func (c *Client) ListScheduledWithParams(ctx context.Context, p ListScheduledParams) ([]Scheduled, error) {
	var out struct {
		Data []Scheduled `json:"data"`
	}
	err := c.do(ctx, http.MethodGet, "/messages/scheduled", newQuery().int("limit", p.Limit).values(), nil, &out)
	return out.Data, err
}

// ListCampaignRecipientsWithParams lists recipients with a limit.
// GET /campaigns/{id}/recipients?limit=.
func (c *Client) ListCampaignRecipientsWithParams(ctx context.Context, id string, p ListCampaignRecipientsParams) ([]CampaignRecipient, error) {
	var out struct {
		Data []CampaignRecipient `json:"data"`
	}
	err := c.do(ctx, http.MethodGet, "/campaigns/"+seg(id)+"/recipients", newQuery().int("limit", p.Limit).values(), nil, &out)
	return out.Data, err
}

// TriggerWebhookEvent emits a sample event to the project's webhooks (to test
// your endpoint end to end). POST /webhooks/trigger.
func (c *Client) TriggerWebhookEvent(ctx context.Context, eventType string) (*WebhookEventRef, error) {
	return call[WebhookEventRef](ctx, c, apiRequest{method: http.MethodPost, path: "/webhooks/trigger",
		body: struct {
			EventType string `json:"event_type,omitempty"`
		}{eventType}})
}
