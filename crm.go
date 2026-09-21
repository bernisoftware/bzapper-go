package bzapper

import (
	"context"
	"net/http"
)

// --- Contacts (CRM): the project's contact base, tags, groups, opt-in/out and
// suppressions. The contact↔project/number link is maintained automatically by
// the API. ---

// ContactAddress is a contact's postal address.
type ContactAddress struct {
	Street     string `json:"street,omitempty"`
	Number     string `json:"number,omitempty"`
	Complement string `json:"complement,omitempty"`
	District   string `json:"district,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	Zip        string `json:"zip,omitempty"`
	Country    string `json:"country,omitempty"`
}

// CreateContactParams is the request for CreateContact. Phone is required.
type CreateContactParams struct {
	Phone        string          `json:"phone"`
	Name         string          `json:"name,omitempty"`
	Email        string          `json:"email,omitempty"`
	Document     string          `json:"document,omitempty"`
	DocumentType string          `json:"document_type,omitempty"`
	Address      *ContactAddress `json:"address,omitempty"`
}

// UpdateContactParams is the request for UpdateContact. Nil fields are not
// sent (left unchanged).
type UpdateContactParams struct {
	Name         *string         `json:"name,omitempty"`
	Email        *string         `json:"email,omitempty"`
	Document     *string         `json:"document,omitempty"`
	DocumentType *string         `json:"document_type,omitempty"`
	Address      *ContactAddress `json:"address,omitempty"`
}

// ContactHistoryItem is one entry of a contact timeline (a message or an event).
type ContactHistoryItem struct {
	Kind      string         `json:"kind"` // "message" | "event"
	Type      string         `json:"type,omitempty"`
	Direction string         `json:"direction,omitempty"` // "", "inbound", "outbound"
	Status    string         `json:"status,omitempty"`
	Actor     string         `json:"actor,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt string         `json:"created_at"`
}

// ContactHistoryList is the response of GetContactHistory.
type ContactHistoryList struct {
	Data []ContactHistoryItem `json:"data"`
}

// ContactHistoryParams is the optional query of GetContactHistory.
type ContactHistoryParams struct {
	Limit int
}

// TaxonMutation adds and/or removes tag (or contact-group) keys of a contact.
type TaxonMutation struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// Taxon is a tag or a contact group.
type Taxon struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	Color     string `json:"color,omitempty"`
	Count     int    `json:"count,omitempty"` // contacts with it
	CreatedAt string `json:"created_at,omitempty"`
}

// TaxonList is the response of ListTags and ListContactGroups.
type TaxonList struct {
	Data []Taxon `json:"data"`
}

// CreateTaxonParams creates a tag or a contact group. Key is required.
type CreateTaxonParams struct {
	Key   string `json:"key"`
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

// TaxonRef is the response of CreateTag and CreateContactGroup.
type TaxonRef struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// Suppression is a phone that never receives campaign/bulk sends.
type Suppression struct {
	ID        string `json:"id"`
	Phone     string `json:"phone"`
	Reason    string `json:"reason,omitempty"`
	Source    string `json:"source,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// SuppressionList is the response of ListSuppressions.
type SuppressionList struct {
	Data []Suppression `json:"data"`
}

// ListSuppressionsParams is the optional query of ListSuppressions.
type ListSuppressionsParams struct {
	Limit int
}

// CreateSuppressionParams is the request for CreateSuppression.
type CreateSuppressionParams struct {
	Phone  string `json:"phone"`
	Reason string `json:"reason,omitempty"`
}

// CreateContact creates a contact in the project. POST /contacts.
func (c *Client) CreateContact(ctx context.Context, p CreateContactParams) (*ContactRecord, error) {
	return call[ContactRecord](ctx, c, apiRequest{method: http.MethodPost, path: "/contacts", body: p})
}

// GetContact fetches a contact. GET /contacts/{id}.
func (c *Client) GetContact(ctx context.Context, id string) (*ContactRecord, error) {
	return call[ContactRecord](ctx, c, apiRequest{method: http.MethodGet, path: "/contacts/" + seg(id)})
}

// UpdateContact updates a contact (nil fields are left unchanged).
// PATCH /contacts/{id}.
func (c *Client) UpdateContact(ctx context.Context, id string, p UpdateContactParams) (*ContactRecord, error) {
	return call[ContactRecord](ctx, c, apiRequest{method: http.MethodPatch, path: "/contacts/" + seg(id), body: p})
}

// DeleteContact deletes a contact. DELETE /contacts/{id}.
func (c *Client) DeleteContact(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/contacts/" + seg(id)})
}

// GetContactHistory returns the contact timeline (messages and events).
// GET /contacts/{id}/history.
func (c *Client) GetContactHistory(ctx context.Context, id string, p ContactHistoryParams) (*ContactHistoryList, error) {
	return call[ContactHistoryList](ctx, c, apiRequest{method: http.MethodGet, path: "/contacts/" + seg(id) + "/history",
		query: newQuery().int("limit", p.Limit).values()})
}

// AddContactNote adds a note to the contact timeline. POST /contacts/{id}/notes.
func (c *Client) AddContactNote(ctx context.Context, id, body string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/contacts/" + seg(id) + "/notes",
		body: struct {
			Body string `json:"body"`
		}{body}})
}

// MutateContactTags adds/removes tag keys of a contact. POST /contacts/{id}/tags.
func (c *Client) MutateContactTags(ctx context.Context, id string, p TaxonMutation) (*ContactRecord, error) {
	return call[ContactRecord](ctx, c, apiRequest{method: http.MethodPost, path: "/contacts/" + seg(id) + "/tags", body: p})
}

// MutateContactGroups adds/removes contact-group keys of a contact.
// POST /contacts/{id}/groups.
func (c *Client) MutateContactGroups(ctx context.Context, id string, p TaxonMutation) (*ContactRecord, error) {
	return call[ContactRecord](ctx, c, apiRequest{method: http.MethodPost, path: "/contacts/" + seg(id) + "/groups", body: p})
}

// OptOutContact marks the contact as opted out (no bulk sends).
// POST /contacts/{id}/optout.
func (c *Client) OptOutContact(ctx context.Context, id string) (*ContactRecord, error) {
	return call[ContactRecord](ctx, c, apiRequest{method: http.MethodPost, path: "/contacts/" + seg(id) + "/optout"})
}

// SuppressContact adds the contact's phone to the suppression list.
// POST /contacts/{id}/suppress.
func (c *Client) SuppressContact(ctx context.Context, id string) (*ContactRecord, error) {
	return call[ContactRecord](ctx, c, apiRequest{method: http.MethodPost, path: "/contacts/" + seg(id) + "/suppress"})
}

// OptInContact reactivates an opted-out contact. POST /contacts/{id}/optin.
func (c *Client) OptInContact(ctx context.Context, id string) (*ContactRecord, error) {
	return call[ContactRecord](ctx, c, apiRequest{method: http.MethodPost, path: "/contacts/" + seg(id) + "/optin"})
}

// ListTags lists the project's tags. GET /tags.
func (c *Client) ListTags(ctx context.Context) (*TaxonList, error) {
	return call[TaxonList](ctx, c, apiRequest{method: http.MethodGet, path: "/tags"})
}

// CreateTag creates a tag. POST /tags.
func (c *Client) CreateTag(ctx context.Context, p CreateTaxonParams) (*TaxonRef, error) {
	return call[TaxonRef](ctx, c, apiRequest{method: http.MethodPost, path: "/tags", body: p})
}

// DeleteTag deletes a tag. DELETE /tags/{id}.
func (c *Client) DeleteTag(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/tags/" + seg(id)})
}

// ListContactGroups lists the project's contact groups. GET /contact-groups.
func (c *Client) ListContactGroups(ctx context.Context) (*TaxonList, error) {
	return call[TaxonList](ctx, c, apiRequest{method: http.MethodGet, path: "/contact-groups"})
}

// CreateContactGroup creates a contact group. POST /contact-groups.
func (c *Client) CreateContactGroup(ctx context.Context, p CreateTaxonParams) (*TaxonRef, error) {
	return call[TaxonRef](ctx, c, apiRequest{method: http.MethodPost, path: "/contact-groups", body: p})
}

// DeleteContactGroup deletes a contact group. DELETE /contact-groups/{id}.
func (c *Client) DeleteContactGroup(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/contact-groups/" + seg(id)})
}

// ListSuppressions lists the suppressed phones. GET /suppressions.
func (c *Client) ListSuppressions(ctx context.Context, p ListSuppressionsParams) (*SuppressionList, error) {
	return call[SuppressionList](ctx, c, apiRequest{method: http.MethodGet, path: "/suppressions",
		query: newQuery().int("limit", p.Limit).values()})
}

// CreateSuppression suppresses a phone. POST /suppressions.
func (c *Client) CreateSuppression(ctx context.Context, p CreateSuppressionParams) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/suppressions", body: p})
}

// DeleteSuppression removes a phone from the suppression list.
// DELETE /suppressions?phone=.
func (c *Client) DeleteSuppression(ctx context.Context, phone string) error {
	if phone == "" {
		return argErr("phone must not be empty (DeleteSuppression)")
	}
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/suppressions",
		query: newQuery().str("phone", phone).values()})
}
