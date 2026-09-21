package bzapper

// Test support: a fake HTTP server (net/http/httptest) that answers the queued
// responses in order and records each request exactly as it arrived (raw path
// from the request line, query, headers, body); and a client whose retry waits
// only get recorded — the suite never really sleeps.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeResponse is a queued response (the cases.json format): Body null/absent =
// no body; a JSON string = text/plain body; anything else goes as JSON.
type fakeResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

// recorded is a request received by the fake server.
type recorded struct {
	Method   string
	Path     string // RAW path, as in the request line (not decoded)
	RawQuery string
	Query    url.Values
	Header   http.Header
	Body     []byte
}

func (r recorded) has(header string) bool {
	_, ok := r.Header[http.CanonicalHeaderKey(header)]
	return ok
}

type fakeServer struct {
	*httptest.Server
	mu        sync.Mutex
	responses []fakeResponse
	requests  []recorded
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	fs := &fakeServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(fs.handle))
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fakeServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	path, rawQuery, _ := strings.Cut(r.RequestURI, "?")
	q, _ := url.ParseQuery(rawQuery)

	fs.mu.Lock()
	fs.requests = append(fs.requests, recorded{
		Method: r.Method, Path: path, RawQuery: rawQuery, Query: q,
		Header: r.Header.Clone(), Body: body,
	})
	var resp *fakeResponse
	if len(fs.responses) > 0 {
		resp = &fs.responses[0]
		fs.responses = fs.responses[1:]
	}
	fs.mu.Unlock()

	if resp == nil {
		// An unexpected extra request: 418 is never retried and the test flags
		// it by the request count.
		resp = &fakeResponse{Status: 418, Body: json.RawMessage(`{"code":"UNEXPECTED_REQUEST","message":"unexpected request"}`)}
	}
	var payload []byte
	contentType := ""
	switch raw := bytes.TrimSpace(resp.Body); {
	case len(raw) == 0 || string(raw) == "null":
	case raw[0] == '"':
		var s string
		_ = json.Unmarshal(raw, &s)
		payload, contentType = []byte(s), "text/plain; charset=utf-8"
	default:
		payload, contentType = raw, "application/json"
	}
	h := w.Header()
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	for k, v := range resp.Headers {
		h.Set(k, v)
	}
	if resp.Status != http.StatusNoContent {
		h.Set("Content-Length", strconv.Itoa(len(payload)))
	}
	w.WriteHeader(resp.Status)
	_, _ = w.Write(payload)
}

// reset queues the responses and clears the recorded requests.
func (fs *fakeServer) reset(responses ...fakeResponse) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.responses = append([]fakeResponse(nil), responses...)
	fs.requests = nil
}

func (fs *fakeServer) recorded() []recorded {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return append([]recorded(nil), fs.requests...)
}

// sleepLog replaces the wait between retries: it only records (never sleeps).
type sleepLog struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (s *sleepLog) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.waits = append(s.waits, d)
	s.mu.Unlock()
	return ctx.Err()
}

func (s *sleepLog) all() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.waits...)
}

// fakeClient points a client at the fake server with the waits disabled.
func fakeClient(t *testing.T, fs *fakeServer, responses []fakeResponse, opts ...Option) (*Client, *sleepLog) {
	t.Helper()
	fs.reset(responses...)
	c := New(fs.URL, "bz_live_unit", opts...)
	log := &sleepLog{}
	c.hooks.sleep = log.sleep
	return c, log
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func jsonResp(status int, body string, headers map[string]string) fakeResponse {
	return fakeResponse{Status: status, Headers: headers, Body: json.RawMessage(body)}
}

func textResp(status int, body string, headers map[string]string) fakeResponse {
	return fakeResponse{Status: status, Headers: headers, Body: mustJSON(body)}
}
