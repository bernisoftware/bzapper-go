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
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Version is the SDK version. Bumped by scripts/release-sdks.sh; the publish
// workflow refuses to release when it disagrees with the git tag (the tag is
// the source of truth for Go modules).
//
// Not cosmetic: it goes in the X-Bzapper-Client header of every request, which
// is how the API knows who to warn when a fix requires updating integration code.
const Version = "0.7.1"

// ClientID identifies the SDK and version to the API (X-Bzapper-Client / User-Agent).
const ClientID = "bzapper-go/" + Version

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
	projectID  string
	maxRetries int
	httpClient *http.Client

	// hooks holds the retry sleep behind a pointer so Client stays comparable.
	hooks *clientHooks
}

// clientHooks: sleep waits between retries. The package tests swap it for one
// that only records the wait — the conformance suite never really sleeps.
type clientHooks struct {
	sleep func(ctx context.Context, d time.Duration) error
}

// Option customizes a Client. Pass options to New.
type Option func(*Client)

// WithLocale sets the Accept-Language header sent on every request
// (e.g. "pt-BR"). Error messages are returned in this locale.
func WithLocale(locale string) Option {
	return func(c *Client) { c.locale = locale }
}

// WithMaxRetries sets how many times a failed call is retried after the first
// attempt (default DefaultMaxRetries = 2; 0 disables retries; negative = 0).
// Only network errors/timeouts and 429, 502, 503 and 504 are retried, with the
// same X-Request-Id and Idempotency-Key, honoring Retry-After (capped at 60s)
// or else an exponential backoff with jitter.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n < 0 {
			n = 0
		}
		c.maxRetries = n
	}
}

// WithProjectID sends X-Project-Id on every request, scoping the calls to that
// project (a project-bound API key already carries its own).
func WithProjectID(projectID string) Option {
	return func(c *Client) { c.projectID = projectID }
}

// WithTimeout sets the request timeout (per attempt). Ignored if WithHTTPClient is also
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
//
// No network call is made. An empty apiKey is not rejected here (the signature
// predates the rule); every call then fails with an error matching
// ErrInvalidArgument before any request is sent.
func New(baseURL, apiKey string, opts ...Option) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		maxRetries: DefaultMaxRetries,
		hooks:      &clientHooks{sleep: sleepContext},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: DefaultTimeout}
	} else if c.httpClient.Timeout == 0 {
		c.httpClient.Timeout = DefaultTimeout
	}
	return c
}

// String describes the client WITHOUT the API key — safe to log.
func (c *Client) String() string {
	if c == nil {
		return "bzapper.Client(nil)"
	}
	return fmt.Sprintf("bzapper.Client{baseURL: %q, maxRetries: %d}", c.baseURL, c.maxRetries)
}

// GoString is %#v — also without the API key.
func (c *Client) GoString() string { return c.String() }
