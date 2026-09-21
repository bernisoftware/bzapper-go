package bzapper

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── version ─────────────────────────────────────────────────────────────────

// The pattern scripts/release-sdks.sh uses to bump the Go version (client.go).
var releaseScriptVersionRE = regexp.MustCompile(`(?m)^(const Version = ")([^"]+)"`)

func TestVersionIsSemverAndInClientHeader(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Errorf("Version %q is not MAJOR.MINOR.PATCH", Version)
	}
	if ClientID != "bzapper-go/"+Version {
		t.Errorf("ClientID %q", ClientID)
	}
}

// Go has no package manifest: the published version IS the git tag, and the
// publish workflow refuses a tag that differs from this constant. This test
// locks the line the release script bumps (exactly one match, same value).
func TestVersionLineMatchesReleaseScript(t *testing.T) {
	src, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatal(err)
	}
	m := releaseScriptVersionRE.FindAllStringSubmatch(string(src), -1)
	if len(m) != 1 {
		t.Fatalf("the release-sdks.sh pattern matched %d times in client.go (must be 1)", len(m))
	}
	if m[0][2] != Version {
		t.Errorf("client.go has %q on the release line, Version is %q", m[0][2], Version)
	}
}

// In the monorepo, clients/release.json (when present) is the shared version of
// every SDK.
func TestVersionMatchesMonorepoRelease(t *testing.T) {
	data, err := os.ReadFile("../release.json")
	if err != nil {
		t.Skip("clients/release.json not present")
	}
	var release struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		t.Fatal(err)
	}
	if release.Version != Version {
		t.Errorf("clients/release.json declares %q, client.go has %q", release.Version, Version)
	}
}

func TestGoModDeclaresModuleWithoutDependencies(t *testing.T) {
	src, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	mod := string(src)
	if !regexp.MustCompile(`(?m)^module github\.com/bernisoftware/bzapper-go$`).MatchString(mod) {
		t.Error("go.mod must declare module github.com/bernisoftware/bzapper-go")
	}
	if regexp.MustCompile(`(?m)^require\b`).MatchString(mod) {
		t.Error("zero runtime dependencies: go.mod must not require anything")
	}
}

// ── network errors ──────────────────────────────────────────────────────────

// recordingTransport records every X-Request-Id / Idempotency-Key the SDK sent
// before delegating to the real transport.
type recordingTransport struct {
	mu   sync.Mutex
	ids  []string
	keys []string
}

func (rt *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.ids = append(rt.ids, r.Header.Get("X-Request-Id"))
	rt.keys = append(rt.keys, r.Header.Get("Idempotency-Key"))
	rt.mu.Unlock()
	return http.DefaultTransport.RoundTrip(r)
}

func TestNetworkErrorWhenServerIsDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close() // nothing listens there anymore

	rt := &recordingTransport{}
	c := New(base, "bz_live_unit", WithHTTPClient(&http.Client{Transport: rt}))
	log := &sleepLog{}
	c.hooks.sleep = log.sleep

	_, err := c.SendText(context.Background(), SendTextParams{SendBase: SendBase{To: "+5511999999999"}, Body: "oi"})
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if e.StatusCode != 0 || e.Code != CodeNetworkError || e.Type != ErrorTypeNetwork || !errors.Is(err, ErrNetwork) {
		t.Errorf("got %+v", e)
	}
	if len(rt.ids) != 1+DefaultMaxRetries {
		t.Fatalf("attempts %d, expected %d (network errors are retried)", len(rt.ids), 1+DefaultMaxRetries)
	}
	for i := range rt.ids {
		if rt.ids[i] != rt.ids[0] || rt.keys[i] != rt.keys[0] || rt.keys[0] == "" {
			t.Errorf("attempt %d: ids %q/%q, first %q/%q — must repeat", i, rt.ids[i], rt.keys[i], rt.ids[0], rt.keys[0])
		}
	}
	if e.RequestID != rt.ids[0] {
		t.Errorf("RequestID %q, expected the sent %q", e.RequestID, rt.ids[0])
	}
	if e.Err == nil || !strings.Contains(err.Error(), CodeNetworkError) {
		t.Errorf("cause/message: %v", err)
	}
	if len(log.all()) != DefaultMaxRetries {
		t.Errorf("waits %v", log.all())
	}
}

func TestCallerCancellationIsNotRetried(t *testing.T) {
	fs := newFakeServer(t)
	c, log := fakeClient(t, fs, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.GetInstance(ctx, "i1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if len(log.all()) != 0 {
		t.Errorf("no retry on cancellation: %v", log.all())
	}
}

// ── idempotency ─────────────────────────────────────────────────────────────

func TestUserIdempotencyKeyIsUsedVerbatim(t *testing.T) {
	fs := newFakeServer(t)
	ok := jsonResp(202, `{"message_id":"m1","status":"queued"}`, nil)

	c, _ := fakeClient(t, fs, []fakeResponse{ok})
	if _, err := c.SendText(context.Background(), SendTextParams{SendBase: SendBase{To: "x", IdempotencyKey: "pedido 4471/ç"}, Body: "y"}); err != nil {
		t.Fatal(err)
	}
	if got := fs.recorded()[0].Header.Get("Idempotency-Key"); got != "pedido 4471/ç" {
		t.Errorf("SendBase.IdempotencyKey: header %q", got)
	}

	fs.reset(jsonResp(201, `{"id":"c1","phone":"+5511"}`, nil))
	ctx := ContextWithIdempotencyKey(context.Background(), "contato-42")
	if _, err := c.CreateContact(ctx, CreateContactParams{Phone: "+5511"}); err != nil {
		t.Fatal(err)
	}
	if got := fs.recorded()[0].Header.Get("Idempotency-Key"); got != "contato-42" {
		t.Errorf("ContextWithIdempotencyKey: header %q", got)
	}

	// The same key survives the retries.
	fs.reset(jsonResp(503, `{"code":"unavailable","message":"x"}`, nil), ok)
	if _, err := c.SendText(context.Background(), SendTextParams{SendBase: SendBase{To: "x", IdempotencyKey: "k-1"}, Body: "y"}); err != nil {
		t.Fatal(err)
	}
	for i, r := range fs.recorded() {
		if r.Header.Get("Idempotency-Key") != "k-1" {
			t.Errorf("attempt %d: %q", i, r.Header.Get("Idempotency-Key"))
		}
	}
}

func TestIdempotencyKeyOnlyOnWritesAndNewPerCall(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{
		jsonResp(200, `{"id":"i1","phone":"x","status":"connected"}`, nil),
		jsonResp(204, ``, nil),
		jsonResp(204, ``, nil),
	})
	_, _ = c.GetInstance(context.Background(), "i1")
	_ = c.DeleteInstance(context.Background(), "i1")
	_ = c.DeleteInstance(context.Background(), "i1")
	reqs := fs.recorded()
	if reqs[0].has("Idempotency-Key") {
		t.Error("GET must not carry Idempotency-Key")
	}
	k1, k2 := reqs[1].Header.Get("Idempotency-Key"), reqs[2].Header.Get("Idempotency-Key")
	if k1 == "" || k1 == k2 {
		t.Errorf("each write call needs its own key: %q %q", k1, k2)
	}
	if reqs[1].Header.Get("X-Request-Id") == reqs[2].Header.Get("X-Request-Id") {
		t.Error("each call needs its own X-Request-Id")
	}
}

// ── retries ─────────────────────────────────────────────────────────────────

func TestRetryBackoffAndStatuses(t *testing.T) {
	fs := newFakeServer(t)
	bad := func(status int) fakeResponse { return jsonResp(status, `{"code":"x","message":"x"}`, nil) }
	okBody := jsonResp(200, `{"data":[]}`, nil)

	c, log := fakeClient(t, fs, []fakeResponse{bad(502), bad(504), okBody})
	if _, err := c.ListWebhooks(context.Background()); err != nil {
		t.Fatal(err)
	}
	waits := log.all()
	if len(waits) != 2 || waits[0] < 500*time.Millisecond || waits[0] > 625*time.Millisecond ||
		waits[1] < time.Second || waits[1] > 1250*time.Millisecond {
		t.Errorf("exponential backoff with jitter: %v", waits)
	}

	for _, status := range []int{500, 400, 401, 404, 409, 422} {
		c, log := fakeClient(t, fs, []fakeResponse{bad(status), okBody})
		if _, err := c.ListWebhooks(context.Background()); err == nil {
			t.Errorf("%d: expected error", status)
		}
		if len(fs.recorded()) != 1 || len(log.all()) != 0 {
			t.Errorf("%d must not be retried", status)
		}
	}
}

func TestRetryAfterCappedAndHTTPDate(t *testing.T) {
	fs := newFakeServer(t)
	rl := func(ra string) fakeResponse {
		return jsonResp(429, `{"code":"rate_limited","message":"x"}`, map[string]string{"Retry-After": ra})
	}
	c, log := fakeClient(t, fs, []fakeResponse{rl("600"), rl("2.5"), rl("0")})
	_, err := c.ListWebhooks(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Code != "rate_limited" || !errors.Is(err, ErrRateLimit) || e.RetryAfter != 0 {
		t.Fatalf("got %v (%+v)", err, e)
	}
	if w := log.all(); len(w) != 2 || w[0] != 60*time.Second || w[1] != 2500*time.Millisecond {
		t.Errorf("waits %v", w)
	}

	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if d := parseRetryAfter(now.Add(7*time.Second).Format(http.TimeFormat), now); d == nil || *d != 7*time.Second {
		t.Errorf("HTTP date: %v", d)
	}
	if d := parseRetryAfter("soon", now); d != nil {
		t.Errorf("garbage: %v", d)
	}
}

func TestMaxRetriesZeroDisablesRetries(t *testing.T) {
	fs := newFakeServer(t)
	c, log := fakeClient(t, fs, []fakeResponse{jsonResp(503, `{"code":"x","message":"x"}`, nil)}, WithMaxRetries(0))
	if _, err := c.ListWebhooks(context.Background()); !errors.Is(err, ErrServer) {
		t.Fatalf("got %v", err)
	}
	if len(fs.recorded()) != 1 || len(log.all()) != 0 {
		t.Error("WithMaxRetries(0) must not retry")
	}
}

// ── errors and responses ────────────────────────────────────────────────────

func TestErrorFields(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{jsonResp(403,
		`{"code":"insufficient_scope","message":"Escopo faltando","locale":"pt-BR","detail":{"a":1}}`,
		map[string]string{"X-Required-Scope": "messages:send", "X-Request-Id": "req-9"})})
	_, err := c.GetInstance(context.Background(), "i1")
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("got %T", err)
	}
	if e.Code != "insufficient_scope" || e.Message != "Escopo faltando" || e.Locale != "pt-BR" ||
		e.StatusCode != 403 || e.RequestID != "req-9" || e.RequiredScope != "messages:send" ||
		e.Type != ErrorTypePermissionDenied || !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("got %+v", e)
	}
	if body, ok := e.Body.(map[string]any); !ok || body["detail"] == nil {
		t.Errorf("Body %#v", e.Body)
	}
	if s := err.Error(); !strings.Contains(s, "insufficient_scope") || !strings.Contains(s, "req-9") {
		t.Errorf("Error() %q", s)
	}
}

func TestErrorCodeFallsBackToErrorFieldThenStatus(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{jsonResp(402, `{"error":"payment_required"}`, nil)})
	_, err := c.GetInstance(context.Background(), "i1")
	var e *Error
	if !errors.As(err, &e) || e.Code != "payment_required" || e.Message != "payment_required" || e.Type != ErrorTypeAPI {
		t.Errorf("got %+v", e)
	}
	c, _ = fakeClient(t, fs, []fakeResponse{textResp(404, "not here", nil)})
	_, err = c.GetInstance(context.Background(), "i1")
	if !errors.As(err, &e) || e.Code != "HTTP_404" || e.Body != "not here" || !errors.Is(err, ErrNotFound) {
		t.Errorf("got %+v", e)
	}
}

func TestInvalidSuccessBody(t *testing.T) {
	fs := newFakeServer(t)
	for _, resp := range []fakeResponse{
		textResp(200, "<html>proxy</html>", nil),         // not JSON
		jsonResp(200, `{"id": 123, "phone": true}`, nil), // wrong types
	} {
		c, _ := fakeClient(t, fs, []fakeResponse{resp})
		_, err := c.GetInstance(context.Background(), "i1")
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeInvalidResponse || e.StatusCode != 200 || e.Type != ErrorTypeAPI || e.RequestID == "" {
			t.Errorf("got %v", err)
		}
	}
	// A write that discards the body still refuses a non-JSON 2xx.
	c, _ := fakeClient(t, fs, []fakeResponse{textResp(200, "ok", nil)})
	if err := c.DeleteInstance(context.Background(), "i1"); !strings.Contains(errString(err), CodeInvalidResponse) {
		t.Errorf("got %v", err)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestNoContentReturnsNilOnNewMethods(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{jsonResp(204, ``, nil)})
	got, err := c.GetBlocklist(context.Background(), "i1")
	if err != nil || got != nil {
		t.Errorf("got %v, %v", got, err)
	}
}

// ── arguments ───────────────────────────────────────────────────────────────

func TestInvalidPathParamsFailBeforeAnyRequest(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, nil)
	for _, bad := range []string{"", ".", ".."} {
		if _, err := c.GetInstance(context.Background(), bad); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("GetInstance(%q): %v", bad, err)
		}
		if _, err := c.ConversationHistory(context.Background(), bad, ConversationHistoryParams{}); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("ConversationHistory(%q): %v", bad, err)
		}
		if err := c.MuteChat(context.Background(), bad, "i1", true); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("MuteChat(%q): %v", bad, err)
		}
	}
	if len(fs.recorded()) != 0 {
		t.Errorf("requests were sent: %d", len(fs.recorded()))
	}
}

func TestPathSegmentsArePercentEncoded(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{jsonResp(204, ``, nil), jsonResp(204, ``, nil)})
	_ = c.DeleteLabel(context.Background(), "a/b c", "i1")
	_ = c.MuteChat(context.Background(), "120363@g.us", "i1", true)
	reqs := fs.recorded()
	if reqs[0].Path != "/labels/a%2Fb%20c" {
		t.Errorf("path %q", reqs[0].Path)
	}
	if reqs[1].Path != "/chats/120363@g.us/mute" {
		t.Errorf("path %q", reqs[1].Path)
	}
}

func TestEmptyAPIKeyIsArgumentErrorWithoutRequest(t *testing.T) {
	fs := newFakeServer(t)
	c := New(fs.URL, "  ")
	if _, err := c.ListWebhooks(context.Background()); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("got %v", err)
	}
	if len(fs.recorded()) != 0 {
		t.Error("no request with an empty key")
	}
}

// ── headers, options, uploads ───────────────────────────────────────────────

func TestRequestHeadersAndOptions(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{jsonResp(200, `{"data":[]}`, nil)}, WithLocale("en"), WithProjectID("p-1"))
	if _, err := c.ListTags(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := fs.recorded()[0].Header
	for name, want := range map[string]string{
		"Authorization": "Bearer bz_live_unit", "Accept": "application/json",
		"X-Bzapper-Client": ClientID, "User-Agent": ClientID,
		"Accept-Language": "en", "X-Project-Id": "p-1",
	} {
		if got := h.Get(name); got != want {
			t.Errorf("%s: %q, expected %q", name, got, want)
		}
	}
	if len(h.Get("X-Request-Id")) != 32 {
		t.Errorf("X-Request-Id %q", h.Get("X-Request-Id"))
	}
	if s := c.String(); strings.Contains(s, "bz_live_unit") {
		t.Errorf("String() leaks the key: %s", s)
	}
}

func TestUploadIsMultipart(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{jsonResp(200, `{"url":"https://cdn/x.png"}`, nil)})
	got, err := c.UploadCampaignMedia(context.Background(), UploadFile{Filename: "x.png", Content: []byte("PNG")})
	if err != nil || got.URL != "https://cdn/x.png" {
		t.Fatalf("got %v, %v", got, err)
	}
	r := fs.recorded()[0]
	mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" {
		t.Fatalf("Content-Type %q", r.Header.Get("Content-Type"))
	}
	mr := multipart.NewReader(strings.NewReader(string(r.Body)), params["boundary"])
	part, err := mr.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() != "x.png" || part.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("part %v %v", part, err)
	}
	if r.Header.Get("Idempotency-Key") == "" {
		t.Error("upload is a write: needs Idempotency-Key")
	}
	if _, err := c.UploadCampaignMedia(context.Background(), UploadFile{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("empty filename: %v", err)
	}
}

func TestListContactsSendsEveryFilter(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{jsonResp(200, `{"data":[],"total":0,"limit":10,"offset":0}`, nil)})
	no := false
	_, err := c.ListContacts(context.Background(), ListContactsParams{Tags: "vip,lead", HasEmail: &no, Offset: 20})
	if err != nil {
		t.Fatal(err)
	}
	q := fs.recorded()[0].Query
	if q.Get("tags") != "vip,lead" || q.Get("has_email") != "false" || q.Get("offset") != "20" || q.Has("groups") {
		t.Errorf("query %v", q)
	}
}
