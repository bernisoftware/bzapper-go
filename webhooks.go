package bzapper

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// --- Webhooks (management) ---
//
// These methods manage webhook subscriptions. To RECEIVE and process the
// delivered events, use the WebhookReceiver / ConstructWebhookEvent helpers in
// webhook.go.

// Webhook is a webhook subscription. The signing secret is never returned by
// list/get — it is shown only once when created or rotated (see CreateWebhook).
type Webhook struct {
	ID           string   `json:"id"`
	URL          string   `json:"url"`
	EventTypes   []string `json:"event_types"`
	NumberFilter string   `json:"number_filter,omitempty"`
	Active       bool     `json:"active"`
	CreatedAt    string   `json:"created_at,omitempty"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
}

// WebhookList is the response of ListWebhooks.
type WebhookList struct {
	Data []Webhook `json:"data"`
}

// CreateWebhookParams is the request for CreateWebhook.
type CreateWebhookParams struct {
	// URL is the HTTPS endpoint that will receive the deliveries (required).
	URL string `json:"url"`
	// Secret signs deliveries. Omit to let the API generate a strong one
	// (returned ONCE in WebhookCreated.Secret).
	Secret string `json:"secret,omitempty"`
	// EventTypes are the subscribed events; empty = all. Each event type can
	// belong to a single webhook (409 on conflict).
	EventTypes []string `json:"event_types,omitempty"`
	// NumberFilter restricts deliveries to one number (instance_id). Optional.
	NumberFilter string `json:"number_filter,omitempty"`
}

// WebhookCreated is the response of CreateWebhook. Secret is the signing secret,
// shown only once and never recoverable — store it securely and pass it to
// NewWebhookReceiver.
type WebhookCreated struct {
	Webhook
	Secret string `json:"secret"`
}

// UpdateWebhookParams is the request for UpdateWebhook. All fields are optional;
// pointer types distinguish "unset" from an explicit zero value. Set Secret to
// "regenerate" to rotate the signing secret (the new one is returned once).
type UpdateWebhookParams struct {
	URL          *string   `json:"url,omitempty"`
	Secret       *string   `json:"secret,omitempty"`
	EventTypes   *[]string `json:"event_types,omitempty"`
	NumberFilter *string   `json:"number_filter,omitempty"`
	Active       *bool     `json:"active,omitempty"`
}

// WebhookTestResult is the response of TestWebhook — the delivery attempt's
// outcome against the configured endpoint.
type WebhookTestResult struct {
	Delivered  bool   `json:"delivered"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// WebhookDelivery is a single delivery attempt for a webhook.
type WebhookDelivery struct {
	ID         string `json:"id"`
	EventID    string `json:"event_id,omitempty"`
	EventType  string `json:"event_type,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// WebhookDeliveryList is the response of WebhookDeliveries.
type WebhookDeliveryList struct {
	Data []WebhookDelivery `json:"data"`
}

// ListWebhooks lists the project's webhooks. GET /webhooks.
func (c *Client) ListWebhooks(ctx context.Context) (*WebhookList, error) {
	var out WebhookList
	if err := c.do(ctx, http.MethodGet, "/webhooks", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateWebhook creates a webhook. The signing secret in WebhookCreated.Secret
// is shown only once. POST /webhooks.
func (c *Client) CreateWebhook(ctx context.Context, p CreateWebhookParams) (*WebhookCreated, error) {
	var out WebhookCreated
	if err := c.do(ctx, http.MethodPost, "/webhooks", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateWebhook updates or pauses a webhook. Set p.Secret to "regenerate" to
// rotate the signing secret. PATCH /webhooks/{id}.
func (c *Client) UpdateWebhook(ctx context.Context, id string, p UpdateWebhookParams) (*Webhook, error) {
	var out Webhook
	if err := c.do(ctx, http.MethodPatch, "/webhooks/"+url.PathEscape(id), nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteWebhook deletes a webhook by id. DELETE /webhooks/{id}.
func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/webhooks/"+url.PathEscape(id), nil, nil, nil)
}

// TestWebhook sends a test event to the webhook's endpoint and returns the
// delivery outcome. Pass an empty eventType to use the server default.
// POST /webhooks/{id}/test.
func (c *Client) TestWebhook(ctx context.Context, id, eventType string) (*WebhookTestResult, error) {
	body := struct {
		EventType string `json:"event_type,omitempty"`
	}{EventType: eventType}
	var out WebhookTestResult
	if err := c.do(ctx, http.MethodPost, "/webhooks/"+url.PathEscape(id)+"/test", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WebhookDeliveries returns the recent delivery attempts for a webhook. Pass
// limit <= 0 to use the server default. GET /webhooks/{id}/deliveries.
func (c *Client) WebhookDeliveries(ctx context.Context, id string, limit int) (*WebhookDeliveryList, error) {
	var q url.Values
	if limit > 0 {
		q = url.Values{"limit": {strconv.Itoa(limit)}}
	}
	var out WebhookDeliveryList
	if err := c.do(ctx, http.MethodGet, "/webhooks/"+url.PathEscape(id)+"/deliveries", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
