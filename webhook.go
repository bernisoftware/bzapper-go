package bzapper

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
)

// Webhook delivery headers set by the bZapper API on every delivery.
const (
	// SignatureHeader carries "sha256=<hex>", the HMAC-SHA256 of the raw body.
	SignatureHeader = "X-Bzapper-Signature"
	// EventIDHeader carries the stable, unique event id (use for idempotency).
	EventIDHeader = "X-Bzapper-Event-Id"
	// EventTypeHeader carries the event type (e.g. "message.received").
	EventTypeHeader = "X-Bzapper-Event-Type"
)

// WebhookEventTypes lists every event type the API can deliver, for reference.
var WebhookEventTypes = []string{
	"message.received", "message.sent", "message.delivered", "message.read", "message.failed",
	"instance.connected", "instance.disconnected", "instance.banned", "instance.logged_out",
	"instance.warming", "instance.status",
	"group.joined", "group.participant_added", "group.participant_removed",
	"group.participant_promoted", "group.participant_demoted",
	"group.subject_changed", "group.description_changed",
}

// ErrInvalidSignature is returned when a webhook signature is missing or does
// not match the HMAC of the raw body. Never process such a delivery.
var ErrInvalidSignature = errors.New("bzapper: invalid webhook signature")

// WebhookGroup is the WhatsApp group context, when the event happened in a group.
type WebhookGroup struct {
	JID  string `json:"jid,omitempty"`
	Name string `json:"name,omitempty"`
}

// WebhookSender identifies who sent/triggered the event (message/group events).
type WebhookSender struct {
	JID  string `json:"jid,omitempty"`
	LID  string `json:"lid,omitempty"`
	Name string `json:"name,omitempty"`
}

// WebhookEvent is a parsed, typed webhook event (the delivered envelope). Use ID
// for idempotency — the API may retry deliveries.
type WebhookEvent struct {
	ID              string          `json:"-"`
	Type            string          `json:"-"`
	Timestamp       string          `json:"-"`
	InstanceID      string          `json:"-"`
	ClientReference string          `json:"-"`
	Group           *WebhookGroup   `json:"-"`
	Sender          *WebhookSender  `json:"-"`
	Mentions        []string        `json:"-"`
	Payload         map[string]any  `json:"-"`
	// Raw is the original JSON envelope as delivered.
	Raw json.RawMessage `json:"-"`
}

// webhookEnvelope mirrors the wire format (snake_case) and is mapped onto the
// public WebhookEvent in UnmarshalJSON.
type webhookEnvelope struct {
	EventID         string         `json:"event_id"`
	EventType       string         `json:"event_type"`
	Timestamp       string         `json:"timestamp"`
	InstanceID      string         `json:"instance_id"`
	ClientReference string         `json:"client_reference"`
	Group           *WebhookGroup  `json:"group"`
	Sender          *WebhookSender `json:"sender"`
	Mentions        []string       `json:"mentions"`
	Payload         map[string]any `json:"payload"`
}

// UnmarshalJSON maps the snake_case wire envelope onto the WebhookEvent fields
// and preserves the original bytes in Raw.
func (e *WebhookEvent) UnmarshalJSON(data []byte) error {
	var env webhookEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	e.ID = env.EventID
	e.Type = env.EventType
	e.Timestamp = env.Timestamp
	e.InstanceID = env.InstanceID
	e.ClientReference = env.ClientReference
	e.Group = env.Group
	e.Sender = env.Sender
	e.Mentions = env.Mentions
	e.Payload = env.Payload
	e.Raw = append(e.Raw[:0], data...)
	return nil
}

// signBody computes the expected signature header value for a raw body.
func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhook reports whether signature matches the HMAC-SHA256 of the raw
// body. Comparison is timing-safe. Pass the exact bytes received — never the
// re-serialized JSON.
func VerifyWebhook(secret string, body []byte, signature string) bool {
	if signature == "" {
		return false
	}
	expected := signBody(secret, body)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ConstructWebhookEvent verifies the signature and parses the raw body into a
// WebhookEvent. It returns ErrInvalidSignature if the signature is missing or
// invalid (do NOT process), or a JSON error if the body cannot be parsed.
func ConstructWebhookEvent(secret string, body []byte, signature string) (*WebhookEvent, error) {
	if !VerifyWebhook(secret, body, signature) {
		return nil, ErrInvalidSignature
	}
	var ev WebhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// WebhookHandler processes a parsed webhook event.
type WebhookHandler func(*WebhookEvent)

// WebhookReceiver verifies, parses and routes incoming webhook deliveries to
// handlers registered per event type. It implements http.Handler, so it can be
// mounted directly:
//
//	http.Handle("/webhooks", bzapper.NewWebhookReceiver(secret).
//		On("message.received", func(e *bzapper.WebhookEvent) { ... }))
//
// It is safe for concurrent use; register handlers before serving.
type WebhookReceiver struct {
	secret   string
	mu       sync.RWMutex
	handlers map[string][]WebhookHandler
	any      []WebhookHandler
}

// NewWebhookReceiver creates a receiver that verifies deliveries with secret
// (the webhook's signing secret, returned once by CreateWebhook).
func NewWebhookReceiver(secret string) *WebhookReceiver {
	return &WebhookReceiver{
		secret:   secret,
		handlers: make(map[string][]WebhookHandler),
	}
}

// On registers a handler for an event type. Returns the receiver for chaining.
func (r *WebhookReceiver) On(eventType string, handler WebhookHandler) *WebhookReceiver {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[eventType] = append(r.handlers[eventType], handler)
	return r
}

// OnAny registers a handler that runs for every event. Returns the receiver for
// chaining.
func (r *WebhookReceiver) OnAny(handler WebhookHandler) *WebhookReceiver {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.any = append(r.any, handler)
	return r
}

// Handle verifies, parses and dispatches a delivery to the matching handlers
// (the event-type handlers, then the OnAny handlers). It returns the parsed
// event (use Event.ID for idempotency) or ErrInvalidSignature when the
// signature is invalid — in which case no handler runs.
func (r *WebhookReceiver) Handle(body []byte, signature string) (*WebhookEvent, error) {
	ev, err := ConstructWebhookEvent(r.secret, body, signature)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	typed := append([]WebhookHandler(nil), r.handlers[ev.Type]...)
	anyH := append([]WebhookHandler(nil), r.any...)
	r.mu.RUnlock()
	for _, h := range typed {
		h(ev)
	}
	for _, h := range anyH {
		h(ev)
	}
	return ev, nil
}

// ServeHTTP implements http.Handler. It reads the raw body, takes the signature
// from the X-Bzapper-Signature header, dispatches via Handle, and responds 200
// on success or 400 on an invalid signature / malformed body.
func (r *WebhookReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}
	signature := req.Header.Get(SignatureHeader)
	if _, err := r.Handle(body, signature); err != nil {
		if errors.Is(err, ErrInvalidSignature) {
			http.Error(w, "invalid signature", http.StatusBadRequest)
			return
		}
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}
