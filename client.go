// Package bzapper is the official Go SDK for the bZapper API — a multi-tenant
// WhatsApp gateway. It wraps the REST API documented in
// packages/sdk/openapi.yaml using only the standard library (net/http +
// encoding/json).
//
// Quick start (points at production — just pass your API key):
//
//	client := bzapper.NewClient("bz_live_...")
//	msg, err := client.SendText(context.Background(), bzapper.SendTextParams{
//		To:   "+5511999999999",
//		Body: "Olá do bZapper!",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println("queued:", msg.MessageID)
package bzapper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Version is the SDK version.
const Version = "0.5.0"

// DefaultBaseURL is the production API. NewClient uses it by default; override
// only in dev/self-host via WithBaseURL (or the New(baseURL, ...) constructor).
const DefaultBaseURL = "https://api.bzapper.com.br"

// DefaultTimeout is applied when no custom http.Client or timeout is provided.
const DefaultTimeout = 30 * time.Second

// Client is the bZapper API client. Create one with New and reuse it; it is
// safe for concurrent use by multiple goroutines.
type Client struct {
	baseURL    string
	apiKey     string
	locale     string
	httpClient *http.Client
}

// Option customizes a Client. Pass options to New.
type Option func(*Client)

// WithLocale sets the Accept-Language header sent on every request
// (e.g. "pt-BR"). Error messages are returned in this locale.
func WithLocale(locale string) Option {
	return func(c *Client) { c.locale = locale }
}

// WithTimeout sets the request timeout. Ignored if WithHTTPClient is also
// provided.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if c.httpClient == nil {
			c.httpClient = &http.Client{}
		}
		c.httpClient.Timeout = d
	}
}

// WithHTTPClient supplies a custom *http.Client (e.g. with a proxy or custom
// transport). Takes precedence over WithTimeout.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBaseURL overrides the API base URL (default: DefaultBaseURL, production).
// Use it with NewClient for dev ("http://localhost:8080") or self-host.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(baseURL, "/") }
}

// NewClient creates a Client pointing at the production API — pass just your API
// key. This is the recommended constructor:
//
//	client := bzapper.NewClient("bz_live_...")
//
// Override the URL only for dev/self-host: bzapper.NewClient("bz_live_...", bzapper.WithBaseURL("http://localhost:8080")).
func NewClient(apiKey string, opts ...Option) *Client {
	return New(DefaultBaseURL, apiKey, opts...)
}

// New creates a Client with an explicit base URL. Prefer NewClient (which
// defaults to production). An empty baseURL falls back to DefaultBaseURL.
//
//   - baseURL: e.g. "https://api.bzapper.com.br" or "http://localhost:8080" ("" = production).
//   - apiKey:  tenant API key, e.g. "bz_live_...".
func New(baseURL, apiKey string, opts ...Option) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: DefaultTimeout}
	} else if c.httpClient.Timeout == 0 {
		c.httpClient.Timeout = DefaultTimeout
	}
	return c
}

// do performs an HTTP request. body is JSON-encoded when non-nil. On a 2xx
// response, out (when non-nil) is JSON-decoded from the body. On a non-2xx
// response a *Error is returned.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("bzapper: encode request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return fmt.Errorf("bzapper: build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.locale != "" {
		req.Header.Set("Accept-Language", c.locale)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bzapper: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("bzapper: read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseError(resp.StatusCode, respBody)
	}

	if out == nil || len(bytes.TrimSpace(respBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("bzapper: decode response body: %w", err)
	}
	return nil
}
