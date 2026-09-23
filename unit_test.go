package bzapper

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
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

// ── CSV export (out of the generated cases: it is not JSON — BRIEF §6) ───────

// The CSV of the fixture has what breaks a naive implementation: a field with a
// comma and doubled quotes, an empty field, a trailing newline — and it is not
// valid JSON.
const exportCSVFixture = "phone,name,email,status,source,tags,groups,created_at,last_activity_at\n" +
	"+5511999990000,Ana,ana@example.com,active,import,vip;lead,,2026-09-01T12:00:00Z,2026-09-20T08:30:00Z\n" +
	"+5511888880000,\"Silva, Maria \"\"Bia\"\"\",,pending_validation,widget,,clientes,2026-09-02T12:00:00Z,\n"

func TestExportContactsReturnsRawCSV(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{textResp(200, exportCSVFixture, map[string]string{
		"Content-Type":        "text/csv; charset=utf-8",
		"Content-Disposition": `attachment; filename="contacts.csv"`,
	})})
	no := false
	got, err := c.ExportContacts(context.Background(), ExportContactsParams{
		Search: "ana", Tags: "vip,lead", TagsMatch: "all", Groups: "clientes", Status: "active",
		City: "São Paulo", State: "SP", Country: "BR", Zip: "01310-000", Document: "12345678901",
		HasEmail: &no, ProjectID: "current", InstanceID: "11111111-1111-4111-8111-111111111111",
		LastActivityAfter: "2026-09-01T00:00:00Z", LastActivityBefore: "2026-09-30T00:00:00Z",
		CreatedAfter: "2026-01-01T00:00:00Z", CreatedBefore: "2026-12-31T00:00:00Z",
		Sort: "name", Limit: 5000,
	})
	if err != nil {
		t.Fatalf("ExportContacts: %v", err)
	}
	// The text comes back byte for byte — no trimming, no re-quoting.
	if got != exportCSVFixture {
		t.Errorf("CSV came back changed:\n%q\nexpected:\n%q", got, exportCSVFixture)
	}

	reqs := fs.recorded()
	if len(reqs) != 1 {
		t.Fatalf("%d requests, expected 1", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodGet || r.Path != "/contacts/export" {
		t.Errorf("%s %s, expected GET /contacts/export", r.Method, r.Path)
	}
	// A GET is not a write: no Idempotency-Key. And the SDK asks for CSV, not JSON —
	// this is what keeps the body out of the JSON path (a JSON Accept would make
	// this very body an INVALID_RESPONSE).
	if accept := r.Header.Get("Accept"); accept != "text/csv" {
		t.Errorf("Accept %q, expected text/csv", accept)
	}
	if r.has("Idempotency-Key") {
		t.Error("GET /contacts/export must not send Idempotency-Key")
	}
	if r.Header.Get("X-Request-Id") == "" {
		t.Error("X-Request-Id missing")
	}
	// The same filters as listContacts, minus the offset (the export has none).
	want := url.Values{
		"search": {"ana"}, "tags": {"vip,lead"}, "tags_match": {"all"}, "groups": {"clientes"},
		"status": {"active"}, "city": {"São Paulo"}, "state": {"SP"}, "country": {"BR"},
		"zip": {"01310-000"}, "document": {"12345678901"}, "has_email": {"false"},
		"project_id": {"current"}, "instance_id": {"11111111-1111-4111-8111-111111111111"},
		"last_activity_after": {"2026-09-01T00:00:00Z"}, "last_activity_before": {"2026-09-30T00:00:00Z"},
		"created_after": {"2026-01-01T00:00:00Z"}, "created_before": {"2026-12-31T00:00:00Z"},
		"sort": {"name"}, "limit": {"5000"},
	}
	if !reflect.DeepEqual(r.Query, want) {
		t.Errorf("query %v, expected %v", r.Query, want)
	}
	if strings.Contains(r.RawQuery, "offset") {
		t.Errorf("the export has no offset parameter: %q", r.RawQuery)
	}
}

// Empty params send no query at all, and an error answer is still a typed
// *Error with the API code (the JSON error body is decoded as usual).
func TestExportContactsNoFiltersAndErrors(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{
		textResp(200, "phone,name\n", map[string]string{"Content-Type": "text/csv"}),
		jsonResp(401, `{"code":"invalid_api_key","message":"chave inválida"}`, nil),
	})
	if _, err := c.ExportContacts(context.Background(), ExportContactsParams{}); err != nil {
		t.Fatalf("ExportContacts: %v", err)
	}
	if raw := fs.recorded()[0].RawQuery; raw != "" {
		t.Errorf("query %q, expected none", raw)
	}
	got, err := c.ExportContacts(context.Background(), ExportContactsParams{})
	if got != "" {
		t.Errorf("error returned text: %q", got)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || !errors.Is(err, ErrAuthentication) || apiErr.Code != "invalid_api_key" {
		t.Fatalf("expected 401 invalid_api_key, got %v", err)
	}
}

func TestImportContactsSendsRowsAndReadsPerRowOutcome(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{jsonResp(200, `{"dry_run":true,"total":2,"created":1,"updated":0,
		"skipped":1,"failed":0,"skipped_rows":[{"index":1,"phone":"5511888880000","reason":"opted_out"}]}`, nil)})
	res, err := c.ImportContacts(context.Background(), ImportContactsParams{
		DryRun: true,
		Contacts: []ContactImportRow{
			{Phone: "+5511999990000", Name: "Ana", Tags: []string{"vip"}, Address: &ContactAddress{City: "São Paulo"}},
			{Phone: "5511888880000"},
		},
	})
	if err != nil {
		t.Fatalf("ImportContacts: %v", err)
	}
	if !res.DryRun || res.Total != 2 || res.Created != 1 || res.Skipped != 1 || len(res.SkippedRows) != 1 ||
		res.SkippedRows[0].Reason != "opted_out" || res.SkippedRows[0].Index != 1 {
		t.Errorf("result %+v", res)
	}
	r := fs.recorded()[0]
	if r.Method != http.MethodPost || r.Path != "/contacts/import" {
		t.Errorf("%s %s", r.Method, r.Path)
	}
	if r.Header.Get("Idempotency-Key") == "" {
		t.Error("an import is a write: needs Idempotency-Key")
	}
	// Only what the caller filled goes on the wire (BRIEF §3).
	var sent map[string]any
	if err := json.Unmarshal(r.Body, &sent); err != nil {
		t.Fatalf("body: %v", err)
	}
	rows, _ := sent["contacts"].([]any)
	if len(rows) != 2 || sent["dry_run"] != true {
		t.Fatalf("body %s", r.Body)
	}
	if second, _ := rows[1].(map[string]any); len(second) != 1 || second["phone"] != "5511888880000" {
		t.Errorf("row without optional fields went as %v", second)
	}
}

func TestRotateKeyGracePeriod(t *testing.T) {
	fs := newFakeServer(t)
	c, _ := fakeClient(t, fs, []fakeResponse{
		jsonResp(200, `{"api_key":"bz_live_new","key":{"id":"k2","tenant_id":"t1","role":"admin"},
			"previous_key":{"id":"k1","tenant_id":"t1","role":"admin","expires_at":"2026-09-23T12:00:00Z","rotated_to":"k2"},
			"old_key_expires_at":"2026-09-23T12:00:00Z"}`, nil),
		jsonResp(200, `{"api_key":"bz_live_new","key":{"id":"k3","tenant_id":"t1","role":"admin"}}`, nil),
	})
	now := 0
	rot, err := c.RotateKey(context.Background(), "k1", RotateKeyParams{RevokeInSeconds: &now})
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	if rot.APIKey != "bz_live_new" || rot.PreviousKey == nil || rot.PreviousKey.RotatedTo != "k2" ||
		rot.PreviousKey.ExpiresAt == nil || *rot.OldKeyExpiresAt != "2026-09-23T12:00:00Z" {
		t.Errorf("result %+v", rot)
	}
	r := fs.recorded()[0]
	if r.Method != http.MethodPost || r.Path != "/keys/k1/rotate" {
		t.Errorf("%s %s", r.Method, r.Path)
	}
	// 0 is meaningful (revoke now), so it MUST be sent.
	if strings.TrimSpace(string(r.Body)) != `{"revoke_in_seconds":0}` {
		t.Errorf("body %s", r.Body)
	}
	// No grace period informed = empty body, the API applies its default.
	if _, err := c.RotateKey(context.Background(), "k3", RotateKeyParams{}); err != nil {
		t.Fatalf("RotateKey without params: %v", err)
	}
	if body := strings.TrimSpace(string(fs.recorded()[1].Body)); body != "{}" {
		t.Errorf("body %s, expected {}", body)
	}
	if _, err := c.RotateKey(context.Background(), "", RotateKeyParams{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("empty id: %v", err)
	}
}
