package bzapper

import (
	"context"
	"net/http"
)

// --- Identity, account and projects ---

// Health is the API liveness (GetHealth).
type Health struct {
	Status  string `json:"status"` // "ok" | "degraded"
	Version string `json:"version"`
}

// MeBFocus carries the signed identity for the bFocus support widget.
type MeBFocus struct {
	UserExternalID     string `json:"user_external_id,omitempty"`
	CustomerExternalID string `json:"customer_external_id,omitempty"`
	UserHash           string `json:"user_hash,omitempty"`
}

// Me is who the API key (or session) belongs to (GetMe).
type Me struct {
	TenantID        string    `json:"tenant_id,omitempty"`
	UserID          string    `json:"user_id,omitempty"`
	Email           string    `json:"email,omitempty"`
	Name            string    `json:"name,omitempty"`
	Phone           string    `json:"phone,omitempty"`
	JobTitle        string    `json:"job_title,omitempty"`
	AvatarURL       string    `json:"avatar_url,omitempty"`
	Role            string    `json:"role,omitempty"`
	Scopes          []string  `json:"scopes,omitempty"`
	Locale          string    `json:"locale,omitempty"`
	TenantName      string    `json:"tenant_name,omitempty"`
	IsPlatformAdmin bool      `json:"is_platform_admin,omitempty"`
	BFocus          *MeBFocus `json:"bfocus,omitempty"`
}

// UpdateProfileParams is the request for UpdateProfile. Nil fields are not sent.
type UpdateProfileParams struct {
	Name     *string `json:"name,omitempty"`
	Phone    *string `json:"phone,omitempty"`
	JobTitle *string `json:"job_title,omitempty"`
	Locale   *string `json:"locale,omitempty"`
}

// UpdateAccountParams is the request for UpdateAccount.
type UpdateAccountParams struct {
	Name string `json:"name"`
}

// AccountUpdated is the response of UpdateAccount.
type AccountUpdated struct {
	TenantName string `json:"tenant_name"`
}

// CreateProjectParams is the request for CreateProjectWithParams. APIMode is
// "UNOFFICIAL" (WhatsApp Web, default) or "OFFICIAL" (WhatsApp Cloud API) and
// cannot be changed later.
type CreateProjectParams struct {
	Name    string `json:"name"`
	APIMode string `json:"api_mode,omitempty"`
}

// UpdateProjectParams is the request for UpdateProject. Nil fields are not sent.
type UpdateProjectParams struct {
	Name    *string `json:"name,omitempty"`
	LogoURL *string `json:"logo_url,omitempty"`
	Color   *string `json:"color,omitempty"`
}

// ProjectHealth is the per-project number-status summary.
type ProjectHealth struct {
	ProjectID string         `json:"project_id"`
	Total     int            `json:"total"`
	Statuses  map[string]int `json:"statuses,omitempty"` // status → count
}

// ProjectHealthList is the response of GetProjectsHealth.
type ProjectHealthList struct {
	Data []ProjectHealth `json:"data"`
}

// LogoUploaded is the response of UploadBrandLogo and UploadProjectLogo.
type LogoUploaded struct {
	LogoURL string `json:"logo_url"`
}

// GetHealth checks the API liveness (no auth needed server-side). GET /healthz.
func (c *Client) GetHealth(ctx context.Context) (*Health, error) {
	return call[Health](ctx, c, apiRequest{method: http.MethodGet, path: "/healthz"})
}

// GetMe returns who the API key belongs to (account, user, role, scopes).
// GET /me.
func (c *Client) GetMe(ctx context.Context) (*Me, error) {
	return call[Me](ctx, c, apiRequest{method: http.MethodGet, path: "/me"})
}

// UpdateProfile updates the user's profile and returns the API response
// ({"user": {...}}). PATCH /me.
func (c *Client) UpdateProfile(ctx context.Context, p UpdateProfileParams) (map[string]any, error) {
	out, err := call[map[string]any](ctx, c, apiRequest{method: http.MethodPatch, path: "/me", body: p})
	if out == nil {
		return nil, err
	}
	return *out, err
}

// UpdateAccount renames the account (admin). PATCH /account.
func (c *Client) UpdateAccount(ctx context.Context, p UpdateAccountParams) (*AccountUpdated, error) {
	return call[AccountUpdated](ctx, c, apiRequest{method: http.MethodPatch, path: "/account", body: p})
}

// CreateProjectWithParams creates a project choosing its API mode (admin).
// CreateProject(ctx, name) remains for the default mode. POST /projects.
func (c *Client) CreateProjectWithParams(ctx context.Context, p CreateProjectParams) (*Project, error) {
	return call[Project](ctx, c, apiRequest{method: http.MethodPost, path: "/projects", body: p})
}

// GetProjectsHealth returns, per project, how many numbers are in each status.
// GET /projects/health.
func (c *Client) GetProjectsHealth(ctx context.Context) (*ProjectHealthList, error) {
	return call[ProjectHealthList](ctx, c, apiRequest{method: http.MethodGet, path: "/projects/health"})
}

// UpdateProject updates a project (admin). PATCH /projects/{id}.
func (c *Client) UpdateProject(ctx context.Context, id string, p UpdateProjectParams) error {
	return c.exec(ctx, apiRequest{method: http.MethodPatch, path: "/projects/" + seg(id), body: p})
}

// DeleteProject deletes a project (admin). DELETE /projects/{id}.
func (c *Client) DeleteProject(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/projects/" + seg(id)})
}

// GetProjectBrand reads a project's number identity. GET /projects/{id}/brand.
func (c *Client) GetProjectBrand(ctx context.Context, id string) (*BrandProfile, error) {
	return call[BrandProfile](ctx, c, apiRequest{method: http.MethodGet, path: "/projects/" + seg(id) + "/brand"})
}

// SetProjectBrand updates a project's number identity. PUT /projects/{id}/brand.
func (c *Client) SetProjectBrand(ctx context.Context, id string, p BrandProfile) (*BrandProfile, error) {
	return call[BrandProfile](ctx, c, apiRequest{method: http.MethodPut, path: "/projects/" + seg(id) + "/brand", body: p})
}

// UploadProjectLogo uploads a project's logo (multipart). POST /projects/{id}/logo.
func (c *Client) UploadProjectLogo(ctx context.Context, id string, file UploadFile) (*LogoUploaded, error) {
	r, err := multipartRequest(http.MethodPost, "/projects/"+seg(id)+"/logo", file, nil)
	if err != nil {
		return nil, err
	}
	return call[LogoUploaded](ctx, c, r)
}

// UploadBrandLogo uploads the brand logo of the active project (multipart).
// POST /brand/logo.
func (c *Client) UploadBrandLogo(ctx context.Context, file UploadFile) (*LogoUploaded, error) {
	r, err := multipartRequest(http.MethodPost, "/brand/logo", file, nil)
	if err != nil {
		return nil, err
	}
	return call[LogoUploaded](ctx, c, r)
}
