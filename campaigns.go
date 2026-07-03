package bzapper

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// --- Scheduled sends -------------------------------------------------------

// Scheduled is a scheduled send (held until ScheduledAt).
type Scheduled struct {
	ID          string `json:"id"`
	ScheduledAt string `json:"scheduled_at"`
	Status      string `json:"status"`
	MessageID   string `json:"message_id,omitempty"`
}

// ListScheduled lists pending/recent scheduled sends. GET /messages/scheduled.
func (c *Client) ListScheduled(ctx context.Context) ([]Scheduled, error) {
	var out struct {
		Data []Scheduled `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/messages/scheduled", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// CancelScheduled cancels a pending scheduled send. DELETE /messages/scheduled/{id}.
func (c *Client) CancelScheduled(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/messages/scheduled/"+url.PathEscape(id), nil, nil, nil)
}

// --- Campaigns (Pro + campaigns add-on) ------------------------------------

// CampaignVariation is a template variation (rotated). Body accepts {variables}
// and spintax {a|b|c}.
type CampaignVariation struct {
	Body   string         `json:"body"`
	Weight int            `json:"weight,omitempty"`
	Media  map[string]any `json:"media,omitempty"`
}

// CampaignCreateParams is the request for CreateCampaign.
type CampaignCreateParams struct {
	Name          string              `json:"name,omitempty"`
	PoolID        string              `json:"pool_id,omitempty"`
	PacingProfile string              `json:"pacing_profile,omitempty"` // conservative|normal
	StartAt       string              `json:"start_at,omitempty"`       // future = scheduled campaign
	Variations    []CampaignVariation `json:"variations"`
}

// Campaign is a campaign.
type Campaign struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Rail          string `json:"rail"`
	PacingProfile string `json:"pacing_profile"`
	PoolID        string `json:"pool_id,omitempty"`
	StartAt       string `json:"start_at,omitempty"`
	PausedReason  string `json:"paused_reason,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// CampaignStats are the aggregated counters of a campaign.
type CampaignStats struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Sent       int `json:"sent"`
	Delivered  int `json:"delivered"`
	Read       int `json:"read"`
	Failed     int `json:"failed"`
	Suppressed int `json:"suppressed"`
}

// CampaignDetail is a campaign with its stats.
type CampaignDetail struct {
	Campaign Campaign      `json:"campaign"`
	Stats    CampaignStats `json:"stats"`
}

// CampaignRecipientInput is one recipient to add: phone + payload of variables.
type CampaignRecipientInput struct {
	Phone   string         `json:"phone"`
	Payload map[string]any `json:"payload,omitempty"`
}

// CampaignRecipient is a campaign recipient with per-contact delivery state.
type CampaignRecipient struct {
	ID          string `json:"id"`
	Phone       string `json:"phone"`
	ContactName string `json:"contact_name,omitempty"`
	Status      string `json:"status"`
	Delivery    string `json:"delivery,omitempty"`
	MessageID   string `json:"message_id,omitempty"`
	LastError   string `json:"last_error,omitempty"`
}

// ContactFilter is a subset of the contacts search filters used to pick
// campaign recipients server-side (only ACTIVE contacts are added).
type ContactFilter struct {
	Search   string   `json:"search,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	TagsAll  bool     `json:"tags_all,omitempty"` // require ALL tags (default any)
	Groups   []string `json:"groups,omitempty"`
	City     string   `json:"city,omitempty"`
	State    string   `json:"state,omitempty"`
	Country  string   `json:"country,omitempty"`
	HasEmail *bool    `json:"has_email,omitempty"`
}

// CampaignRecipientsParams adds recipients. Combine as needed: a Recipients
// array, a Contacts map (phone -> payload), Groups (managed-group slugs
// expanded into individual 1:1 recipients), ContactIDs (explicit contacts)
// and/or ContactFilter (every ACTIVE contact matching a filter). For
// ContactIDs/ContactFilter the phones are resolved server-side. Set Replace
// to swap the whole recipient list instead of appending (draft/scheduled only).
type CampaignRecipientsParams struct {
	Recipients    []CampaignRecipientInput  `json:"recipients,omitempty"`
	Contacts      map[string]map[string]any `json:"contacts,omitempty"`
	Groups        []string                  `json:"groups,omitempty"`
	ContactIDs    []string                  `json:"contact_ids,omitempty"`
	ContactFilter *ContactFilter            `json:"contact_filter,omitempty"`
	Replace       bool                      `json:"replace,omitempty"`
}

// CampaignEstimateParams are the (optional) query filters for EstimateCampaign.
type CampaignEstimateParams struct {
	Recipients int    // number of recipients
	Pacing     string // conservative|normal
	PoolID     string // restrict to a pool
}

// CampaignEstimate is a live send estimate for a recipient count + pacing.
type CampaignEstimate struct {
	Recipients       int    `json:"recipients"`
	NumbersAvailable int    `json:"numbers_available"`
	EstimatedSeconds int    `json:"estimated_seconds"`
	EstimatedHuman   string `json:"estimated_human"`
}

// AddRecipientsResult summarizes a recipients import.
type AddRecipientsResult struct {
	Inserted   int `json:"inserted"`
	Suppressed int `json:"suppressed"`
	Skipped    int `json:"skipped"`
}

// CampaignDryRun is the dry-run simulation result.
type CampaignDryRun struct {
	CampaignID       string        `json:"campaign_id"`
	Stats            CampaignStats `json:"stats"`
	Variations       int           `json:"variations"`
	NumbersAvailable int           `json:"numbers_available"`
	MissingVariables []string      `json:"missing_variables"`
	EstimatedSeconds int           `json:"estimated_seconds"`
	EstimatedHuman   string        `json:"estimated_human"`
	Warnings         []string      `json:"warnings"`
}

// CreateCampaign creates a campaign with template variations. POST /campaigns.
// Requires the Pro plan and the Campaigns add-on.
func (c *Client) CreateCampaign(ctx context.Context, p CampaignCreateParams) (*Campaign, error) {
	var out Campaign
	if err := c.do(ctx, http.MethodPost, "/campaigns", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListCampaigns lists the project's campaigns. GET /campaigns.
func (c *Client) ListCampaigns(ctx context.Context) ([]Campaign, error) {
	var out struct {
		Data []Campaign `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/campaigns", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// GetCampaign gets a campaign with stats. GET /campaigns/{id}.
func (c *Client) GetCampaign(ctx context.Context, id string) (*CampaignDetail, error) {
	var out CampaignDetail
	if err := c.do(ctx, http.MethodGet, "/campaigns/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AddCampaignRecipients adds recipients. POST /campaigns/{id}/recipients.
func (c *Client) AddCampaignRecipients(ctx context.Context, id string, p CampaignRecipientsParams) (*AddRecipientsResult, error) {
	var out AddRecipientsResult
	if err := c.do(ctx, http.MethodPost, "/campaigns/"+url.PathEscape(id)+"/recipients", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListCampaignRecipients lists recipients. GET /campaigns/{id}/recipients.
func (c *Client) ListCampaignRecipients(ctx context.Context, id string) ([]CampaignRecipient, error) {
	var out struct {
		Data []CampaignRecipient `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/campaigns/"+url.PathEscape(id)+"/recipients", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// StartCampaign starts (or schedules) the campaign. POST /campaigns/{id}/start.
func (c *Client) StartCampaign(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/campaigns/"+url.PathEscape(id)+"/start", nil, nil, nil)
}

// PauseCampaign pauses the campaign. POST /campaigns/{id}/pause.
func (c *Client) PauseCampaign(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/campaigns/"+url.PathEscape(id)+"/pause", nil, nil, nil)
}

// ResumeCampaign resumes the campaign. POST /campaigns/{id}/resume.
func (c *Client) ResumeCampaign(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/campaigns/"+url.PathEscape(id)+"/resume", nil, nil, nil)
}

// CancelCampaign cancels the campaign. POST /campaigns/{id}/cancel.
func (c *Client) CancelCampaign(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/campaigns/"+url.PathEscape(id)+"/cancel", nil, nil, nil)
}

// DryRunCampaign simulates without sending. POST /campaigns/{id}/dry-run.
func (c *Client) DryRunCampaign(ctx context.Context, id string) (*CampaignDryRun, error) {
	var out CampaignDryRun
	if err := c.do(ctx, http.MethodPost, "/campaigns/"+url.PathEscape(id)+"/dry-run", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateCampaign edits a not-yet-started (draft/scheduled) campaign, replacing
// the variations when provided. PATCH /campaigns/{id}. 409 once started.
func (c *Client) UpdateCampaign(ctx context.Context, id string, p CampaignCreateParams) (*Campaign, error) {
	var out Campaign
	if err := c.do(ctx, http.MethodPatch, "/campaigns/"+url.PathEscape(id), nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EstimateCampaign returns a live send estimate (eligible numbers + duration)
// for a recipient count and pacing, without creating a campaign. GET /campaigns/estimate.
func (c *Client) EstimateCampaign(ctx context.Context, p CampaignEstimateParams) (*CampaignEstimate, error) {
	q := url.Values{}
	if p.Recipients > 0 {
		q.Set("recipients", strconv.Itoa(p.Recipients))
	}
	if p.Pacing != "" {
		q.Set("pacing", p.Pacing)
	}
	if p.PoolID != "" {
		q.Set("pool_id", p.PoolID)
	}
	var out CampaignEstimate
	if err := c.do(ctx, http.MethodGet, "/campaigns/estimate", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
