package bzapper

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// --- Instances ---

// ListInstances lists the tenant's numbers/instances. GET /instances.
func (c *Client) ListInstances(ctx context.Context) (*InstanceList, error) {
	var out InstanceList
	if err := c.do(ctx, http.MethodGet, "/instances", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateInstance creates a new instance (number). POST /instances.
func (c *Client) CreateInstance(ctx context.Context, p CreateInstanceParams) (*Instance, error) {
	var out Instance
	if err := c.do(ctx, http.MethodPost, "/instances", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetInstance fetches a single instance by id. GET /instances/{id}.
func (c *Client) GetInstance(ctx context.Context, id string) (*Instance, error) {
	var out Instance
	if err := c.do(ctx, http.MethodGet, "/instances/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ConnectInstance starts a connection for an instance using the given method
// (ConnectQR or ConnectCode). Pass an empty method to use the server default
// (qr). POST /instances/{id}/connect.
func (c *Client) ConnectInstance(ctx context.Context, id string, method ConnectMethod) (*ConnectResult, error) {
	var q url.Values
	if method != "" {
		q = url.Values{"method": {string(method)}}
	}
	var out ConnectResult
	if err := c.do(ctx, http.MethodPost, "/instances/"+url.PathEscape(id)+"/connect", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DisconnectInstance disconnects an instance (reconnectable).
// POST /instances/{id}/disconnect.
func (c *Client) DisconnectInstance(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/instances/"+url.PathEscape(id)+"/disconnect", nil, nil, nil)
}

// --- API keys ---

// ListKeys lists the tenant's API keys (without the raw key). GET /keys.
func (c *Client) ListKeys(ctx context.Context) (*APIKeyList, error) {
	var out APIKeyList
	if err := c.do(ctx, http.MethodGet, "/keys", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateKey generates a new API key. The raw key in APIKeyCreated.APIKey is
// shown only once. POST /keys.
func (c *Client) CreateKey(ctx context.Context, p CreateKeyParams) (*APIKeyCreated, error) {
	var out APIKeyCreated
	if err := c.do(ctx, http.MethodPost, "/keys", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeKey revokes an API key by id. DELETE /keys/{id}.
func (c *Client) RevokeKey(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/keys/"+url.PathEscape(id), nil, nil, nil)
}

// --- Usage ---

// GetUsage returns the tenant's usage summary for an optional date range
// (RFC3339). GET /usage.
func (c *Client) GetUsage(ctx context.Context, p GetUsageParams) (*UsageSummary, error) {
	q := url.Values{}
	if p.From != "" {
		q.Set("from", p.From)
	}
	if p.To != "" {
		q.Set("to", p.To)
	}
	var out UsageSummary
	if err := c.do(ctx, http.MethodGet, "/usage", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Account contacts (captured from conversations) ---

// ListContacts lists the account's contact base (captured from conversations).
// p.ProjectID may be a project id or "current". All filters are optional.
// GET /contacts?search=&project_id=&limit=.
func (c *Client) ListContacts(ctx context.Context, p ListContactsParams) (*ContactRecordList, error) {
	q := url.Values{}
	if p.Search != "" {
		q.Set("search", p.Search)
	}
	if p.ProjectID != "" {
		q.Set("project_id", p.ProjectID)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	var out ContactRecordList
	if err := c.do(ctx, http.MethodGet, "/contacts", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Projects ---

// ListProjects lists the account's projects. GET /projects.
func (c *Client) ListProjects(ctx context.Context) (*ProjectList, error) {
	var out ProjectList
	if err := c.do(ctx, http.MethodGet, "/projects", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateProject creates a project (admin). POST /projects.
func (c *Client) CreateProject(ctx context.Context, name string) (*Project, error) {
	body := struct {
		Name string `json:"name"`
	}{Name: name}
	var out Project
	if err := c.do(ctx, http.MethodPost, "/projects", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Brand (number identity kit) ---

// GetBrand reads the project's number identity. GET /brand.
func (c *Client) GetBrand(ctx context.Context) (*BrandProfile, error) {
	var out BrandProfile
	if err := c.do(ctx, http.MethodGet, "/brand", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetBrand updates the project's number identity. PUT /brand.
func (c *Client) SetBrand(ctx context.Context, p BrandProfile) (*BrandProfile, error) {
	var out BrandProfile
	if err := c.do(ctx, http.MethodPut, "/brand", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ApplyBrand applies the "About" to all of the project's connected numbers.
// POST /brand/apply.
func (c *Client) ApplyBrand(ctx context.Context) (*BrandApplyResult, error) {
	var out BrandApplyResult
	if err := c.do(ctx, http.MethodPost, "/brand/apply", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Account users (admin) ---

// ListUsers lists the account's users. GET /users.
func (c *Client) ListUsers(ctx context.Context) (*AccountUserList, error) {
	var out AccountUserList
	if err := c.do(ctx, http.MethodGet, "/users", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// InviteUser invites a user to the account (admin). p.Role is RoleAdmin or
// RoleAgent. POST /users.
func (c *Client) InviteUser(ctx context.Context, p InviteUserParams) (*AccountUser, error) {
	var out AccountUser
	if err := c.do(ctx, http.MethodPost, "/users", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateUserRole changes a user's role (admin). PATCH /users/{id}.
func (c *Client) UpdateUserRole(ctx context.Context, id string, role Role) error {
	body := struct {
		Role Role `json:"role"`
	}{Role: role}
	return c.do(ctx, http.MethodPatch, "/users/"+url.PathEscape(id), nil, body, nil)
}

// RemoveUser removes a user from the account (admin). DELETE /users/{id}.
func (c *Client) RemoveUser(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/users/"+url.PathEscape(id), nil, nil, nil)
}

// GetAccountUsage returns the account's aggregate usage plus a per-project
// breakdown for an optional date range (RFC3339) (admin). GET /account/usage.
func (c *Client) GetAccountUsage(ctx context.Context, p GetUsageParams) (*AccountUsage, error) {
	q := url.Values{}
	if p.From != "" {
		q.Set("from", p.From)
	}
	if p.To != "" {
		q.Set("to", p.To)
	}
	var out AccountUsage
	if err := c.do(ctx, http.MethodGet, "/account/usage", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
