package bzapper

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultMaxRetries is the default number of retries after the first attempt.
const DefaultMaxRetries = 2

// maxRetryAfter caps the wait honored from a Retry-After header.
const maxRetryAfter = 60 * time.Second

// apiRequest is one LOGICAL call: every attempt reuses the same X-Request-Id and
// Idempotency-Key.
type apiRequest struct {
	method string
	path   string // already percent-encoded per segment
	query  url.Values
	// body is JSON-encoded when non-nil (ignored when raw is set).
	body any
	// raw + contentType carry a pre-encoded body (multipart uploads).
	raw         []byte
	contentType string
	// accept overrides the Accept header (empty = application/json). When it is
	// not a JSON media type the 2xx body is NOT parsed nor validated as JSON —
	// that is how the CSV endpoints (ExportContacts) are read (BRIEF §6).
	accept string
	// idempotencyKey is the caller's key (empty = generated on writes).
	idempotencyKey string
}

type idempotencyKeyCtx struct{}

// ContextWithIdempotencyKey returns a context that makes the next write call
// (POST/PUT/PATCH/DELETE) made with it send key, verbatim, as its
// Idempotency-Key header — instead of the key the SDK generates per call.
// Repeating a write with the same key within 24h returns the SAME response
// without executing it again. For the message sends, SendBase.IdempotencyKey
// does the same (and wins when both are set).
//
//	ctx := bzapper.ContextWithIdempotencyKey(ctx, "order-4471")
//	_, err := client.CreateContact(ctx, params)
func ContextWithIdempotencyKey(ctx context.Context, key string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, idempotencyKeyCtx{}, key)
}

func idempotencyKeyFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	k, _ := ctx.Value(idempotencyKeyCtx{}).(string)
	return k
}

// response is a successful (2xx) response.
type response struct {
	status    int
	requestID string
	body      []byte // trimmed; empty = no content
	// rawBody is the body exactly as received (not trimmed) — what the text
	// responses (CSV) must return byte for byte.
	rawBody []byte
}

// do performs a request. body is JSON-encoded when non-nil. On a 2xx response,
// out (when non-nil) is JSON-decoded from the body. Any failure is an *Error
// (API, invalid response or network), an argument error or the context error.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	return c.doWithHeaders(ctx, method, path, query, nil, body, out)
}

// doWithHeaders is do with extra request headers (e.g. Idempotency-Key).
func (c *Client) doWithHeaders(ctx context.Context, method, path string, query url.Values, headers http.Header, body, out any) error {
	r := apiRequest{method: method, path: path, query: query, body: body}
	if headers != nil {
		r.idempotencyKey = headers.Get("Idempotency-Key")
	}
	resp, err := c.send(ctx, r)
	if err != nil {
		return err
	}
	return resp.decode(out)
}

func (r *response) decode(out any) error {
	if out == nil || len(r.body) == 0 {
		return nil
	}
	if err := json.Unmarshal(r.body, out); err != nil {
		return invalidResponse(r.status, r.requestID, err)
	}
	return nil
}

// call performs the request and decodes the body into a new T; a response
// without body (204, or empty 2xx) returns nil.
func call[T any](ctx context.Context, c *Client, r apiRequest) (*T, error) {
	resp, err := c.send(ctx, r)
	if err != nil {
		return nil, err
	}
	if len(resp.body) == 0 || string(resp.body) == "null" {
		return nil, nil
	}
	out := new(T)
	if err := resp.decode(out); err != nil {
		return nil, err
	}
	return out, nil
}

// callText performs the request and returns the response body as text, exactly
// as it came on the wire — no JSON parsing. It is how the endpoints that answer
// text/csv are read (ExportContacts): for them a non-JSON 2xx is the expected
// answer, not an INVALID_RESPONSE (BRIEF §6). Errors (non-2xx) still come back
// as *Error with the API code, decoded from the JSON error body.
func callText(ctx context.Context, c *Client, r apiRequest) (string, error) {
	resp, err := c.send(ctx, r)
	if err != nil {
		return "", err
	}
	return string(resp.rawBody), nil
}

// exec performs the request and discards the body (still validated as JSON).
func (c *Client) exec(ctx context.Context, r apiRequest) error {
	_, err := c.send(ctx, r)
	return err
}

// send performs the logical call with retries (network error, 429, 502, 503,
// 504). X-Request-Id and Idempotency-Key are generated ONCE and repeated on
// every attempt — that is what makes the retry safe.
func (c *Client) send(ctx context.Context, r apiRequest) (*response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, argErr("the API key must not be empty (bzapper.NewClient)")
	}
	if err := validatePath(r.path); err != nil {
		return nil, err
	}
	var payload []byte
	contentType := r.contentType
	switch {
	case r.raw != nil:
		payload = r.raw
	case r.body != nil:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(r.body); err != nil {
			return nil, argErr(fmt.Sprintf("encode request body: %v", err))
		}
		payload = bytes.TrimRight(buf.Bytes(), "\n")
		contentType = "application/json"
	}

	endpoint := c.baseURL + r.path
	if len(r.query) > 0 {
		endpoint += "?" + r.query.Encode()
	}
	requestID := newRequestID()
	idempotencyKey := ""
	if isWrite(r.method) {
		idempotencyKey = r.idempotencyKey
		if idempotencyKey == "" {
			idempotencyKey = idempotencyKeyFrom(ctx)
		}
		if idempotencyKey == "" {
			idempotencyKey = newUUID()
		}
	}

	sleep := sleepContext
	if c.hooks != nil && c.hooks.sleep != nil {
		sleep = c.hooks.sleep
	}
	for attempt := 0; ; attempt++ {
		resp, wait, err := c.attempt(ctx, r.method, endpoint, payload, contentType, r.accept, requestID, idempotencyKey)
		if err == nil {
			return resp, nil
		}
		var apiErr *Error
		if !errors.As(err, &apiErr) || attempt >= c.maxRetries || !apiErr.retryable() {
			return nil, err
		}
		if sleepErr := sleep(ctx, backoff(attempt, wait)); sleepErr != nil {
			return nil, sleepErr
		}
	}
}

// attempt performs ONE HTTP attempt. It returns the response (2xx) or the
// error, plus the wait requested by Retry-After (nil when absent).
func (c *Client) attempt(ctx context.Context, method, endpoint string, payload []byte, contentType, accept, requestID, idempotencyKey string) (*response, *time.Duration, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, nil, argErr(fmt.Sprintf("build request %s %s: %v", method, endpoint, err))
	}
	h := req.Header
	h.Set("Authorization", "Bearer "+c.apiKey)
	if accept == "" {
		accept = "application/json"
	}
	h.Set("Accept", accept)
	// Identifica SDK e versão para a API — é por ele que avisamos você quando a
	// versão que roda tem correção que exige atualizar o código.
	h.Set("X-Bzapper-Client", ClientID)
	h.Set("User-Agent", ClientID)
	h.Set("X-Request-Id", requestID)
	if idempotencyKey != "" {
		h.Set("Idempotency-Key", idempotencyKey)
	}
	if payload != nil && contentType != "" {
		h.Set("Content-Type", contentType)
	}
	if c.locale != "" {
		h.Set("Accept-Language", c.locale)
	}
	if c.projectID != "" {
		h.Set("X-Project-Id", c.projectID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, networkError(ctx, method, endpoint, requestID, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, networkError(ctx, method, endpoint, requestID, err)
	}

	responseID := strings.TrimSpace(resp.Header.Get("X-Request-Id"))
	if responseID == "" {
		responseID = requestID
	}
	trimmed := bytes.TrimSpace(raw)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// A text response (Accept: text/csv) is returned as it came; only a JSON
		// one must really be JSON.
		if strings.Contains(accept, "json") && len(trimmed) > 0 && !json.Valid(trimmed) {
			return nil, nil, invalidResponse(resp.StatusCode, responseID, errors.New("the response body is not JSON"))
		}
		return &response{status: resp.StatusCode, requestID: responseID, body: trimmed, rawBody: raw}, nil, nil
	}
	wait := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	return nil, wait, errorFromResponse(resp.StatusCode, trimmed, resp.Header, wait, responseID)
}

// networkError: a cancellation by the caller returns ctx.Err() (not a network
// error, never retried); anything else — connection refused, attempt timeout —
// is an *Error with Type network, StatusCode 0 and Code NETWORK_ERROR.
func networkError(ctx context.Context, method, endpoint, requestID string, cause error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return &Error{
		Type:      ErrorTypeNetwork,
		Code:      CodeNetworkError,
		Message:   fmt.Sprintf("%s: %s %s failed", CodeNetworkError, method, endpoint),
		RequestID: requestID,
		Err:       cause,
	}
}

// errorFromResponse builds the *Error of a non-2xx response (BRIEF §4).
func errorFromResponse(status int, raw []byte, header http.Header, wait *time.Duration, requestID string) *Error {
	e := &Error{StatusCode: status, Type: typeForStatus(status), RequestID: requestID}
	if len(raw) > 0 {
		var decoded any
		if json.Unmarshal(raw, &decoded) == nil {
			e.Body = decoded
			if obj, ok := decoded.(map[string]any); ok {
				e.Code = stringField(obj, "code")
				if e.Code == "" {
					e.Code = stringField(obj, "error")
				}
				e.Message = stringField(obj, "message")
				e.Locale = stringField(obj, "locale")
			}
		} else {
			e.Body = string(raw)
		}
	}
	if e.Code == "" {
		e.Code = "HTTP_" + strconv.Itoa(status)
	}
	if e.Message == "" {
		e.Message = e.Code
	}
	e.RequiredScope = strings.TrimSpace(header.Get("X-Required-Scope"))
	if status == 429 && wait != nil {
		e.RetryAfter = *wait
	}
	return e
}

func stringField(obj map[string]any, key string) string {
	s, _ := obj[key].(string)
	return s
}

// invalidResponse: a 2xx whose body is not JSON, or does not decode into the
// expected type.
func invalidResponse(status int, requestID string, cause error) *Error {
	return &Error{
		Type:       ErrorTypeAPI,
		Code:       CodeInvalidResponse,
		Message:    CodeInvalidResponse + ": " + cause.Error(),
		StatusCode: status,
		RequestID:  requestID,
		Err:        cause,
	}
}

// validatePath refuses an empty, "." or ".." path segment — the SDK escapes
// every path parameter (url.PathEscape turns "/" into %2F), so such a segment
// can only come from an empty/"."/".." parameter.
func validatePath(path string) error {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, s := range segments {
		switch s {
		case "":
			return argErr(fmt.Sprintf("path parameter must not be empty (%s)", path))
		case ".", "..":
			return argErr(fmt.Sprintf("path parameter must not be %q (%s)", s, path))
		}
	}
	return nil
}

// seg escapes one path parameter (percent-encoding per segment).
func seg(v string) string { return url.PathEscape(v) }

func isWrite(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// backoff is the wait before retry number `attempt` (0 = first retry): the
// Retry-After when present (capped at 60s), else min(8, 0.5 × 2^attempt)s plus
// up to 25% jitter.
func backoff(attempt int, retryAfter *time.Duration) time.Duration {
	if retryAfter != nil {
		d := *retryAfter
		if d < 0 {
			return 0
		}
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		return d
	}
	secs := math.Min(8, 0.5*math.Pow(2, float64(attempt)))
	secs += rand.Float64() * 0.25 * secs
	return time.Duration(secs * float64(time.Second))
}

// parseRetryAfter reads Retry-After in seconds (integer or decimal) or as an
// HTTP date.
func parseRetryAfter(raw string, now time.Time) *time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if secs, err := strconv.ParseFloat(raw, 64); err == nil {
		if secs < 0 || math.IsNaN(secs) || math.IsInf(secs, 0) {
			return nil
		}
		d := time.Duration(math.Min(secs, 1e9) * float64(time.Second))
		return &d
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return &d
	}
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// newRequestID: uuid4 as hex (32 chars).
func newRequestID() string {
	b := uuid4()
	return hex.EncodeToString(b[:])
}

// newUUID: uuid4 in the canonical 8-4-4-4-12 form.
func newUUID() string {
	b := uuid4()
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func uuid4() (b [16]byte) {
	if _, err := crand.Read(b[:]); err != nil {
		for i := range b {
			b[i] = byte(rand.Intn(256))
		}
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return b
}

// --- query helpers ---

// query builds a query string, omitting what was not provided.
type query url.Values

func (q query) str(name, v string) query {
	if v != "" {
		url.Values(q).Set(name, v)
	}
	return q
}

func (q query) int(name string, v int) query {
	if v > 0 {
		url.Values(q).Set(name, strconv.Itoa(v))
	}
	return q
}

func (q query) boolTrue(name string, v bool) query {
	if v {
		url.Values(q).Set(name, "true")
	}
	return q
}

func (q query) boolPtr(name string, v *bool) query {
	if v != nil {
		url.Values(q).Set(name, strconv.FormatBool(*v))
	}
	return q
}

func (q query) values() url.Values {
	if len(q) == 0 {
		return nil
	}
	return url.Values(q)
}

func newQuery() query { return query{} }

// mergeInto copies the pairs into dst.
func (q query) mergeInto(dst url.Values) {
	for k, v := range q {
		dst[k] = v
	}
}
