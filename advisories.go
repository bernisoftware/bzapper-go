package bzapper

import (
	"context"
	"net/http"
	"net/url"
)

// Advisory is an "action required on your integration" notice: a change on our
// side requires you to update YOUR code (an SDK to upgrade, a payload or an
// endpoint that changed).
//
// It is never a changelog. You only receive advisories that affect your account,
// matched against the SDK version you run and the features you actually use.
type Advisory struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Impact is what breaks, concretely.
	Impact string `json:"impact"`
	// Action is what you have to DO.
	Action      string `json:"action"`
	Link        string `json:"link,omitempty"`
	PublishedAt string `json:"published_at"`
}

// AdvisoryList is the response of ListAdvisories.
type AdvisoryList struct {
	Advisories []Advisory `json:"advisories"`
}

// ListAdvisories lists pending integration advisories. GET /advisories.
func (c *Client) ListAdvisories(ctx context.Context) (*AdvisoryList, error) {
	var out AdvisoryList
	if err := c.do(ctx, http.MethodGet, "/advisories", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MarkAdvisoryRead dismisses an advisory once handled.
// POST /advisories/{id}/read.
func (c *Client) MarkAdvisoryRead(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/advisories/"+url.PathEscape(id)+"/read", nil, nil, nil)
}
