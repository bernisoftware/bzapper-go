package bzapper

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSecret = "whsec_test_secret"

func sign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

const samplePayload = `{
	"event_id": "evt_123",
	"event_type": "message.received",
	"timestamp": "2026-06-25T12:00:00Z",
	"instance_id": "inst_1",
	"client_reference": "ref_9",
	"group": {"jid": "123@g.us", "name": "Squad"},
	"sender": {"jid": "55@s.whatsapp.net", "lid": "lid_1", "name": "Ana"},
	"mentions": ["55@s.whatsapp.net"],
	"payload": {"body": "olá"}
}`

func TestHandle_DispatchesAndParses(t *testing.T) {
	rcv := NewWebhookReceiver(testSecret)

	var gotTyped, gotAny *WebhookEvent
	rcv.On("message.received", func(e *WebhookEvent) { gotTyped = e })
	rcv.OnAny(func(e *WebhookEvent) { gotAny = e })
	rcv.On("message.sent", func(e *WebhookEvent) { t.Error("wrong-type handler fired") })

	sig := sign(testSecret, samplePayload)
	ev, err := rcv.Handle([]byte(samplePayload), sig)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gotTyped == nil || gotAny == nil {
		t.Fatal("handlers did not fire")
	}
	if ev.ID != "evt_123" || ev.Type != "message.received" {
		t.Errorf("envelope mapping: id=%q type=%q", ev.ID, ev.Type)
	}
	if ev.InstanceID != "inst_1" || ev.ClientReference != "ref_9" {
		t.Errorf("instance/ref mapping: %+v", ev)
	}
	if ev.Group == nil || ev.Group.Name != "Squad" {
		t.Errorf("group mapping: %+v", ev.Group)
	}
	if ev.Sender == nil || ev.Sender.Name != "Ana" || ev.Sender.LID != "lid_1" {
		t.Errorf("sender mapping: %+v", ev.Sender)
	}
	if len(ev.Mentions) != 1 || ev.Payload["body"] != "olá" {
		t.Errorf("mentions/payload mapping: %+v", ev)
	}
	if len(ev.Raw) == 0 {
		t.Error("Raw not populated")
	}
}

func TestHandle_InvalidSignature(t *testing.T) {
	rcv := NewWebhookReceiver(testSecret)
	fired := false
	rcv.OnAny(func(e *WebhookEvent) { fired = true })

	_, err := rcv.Handle([]byte(samplePayload), "sha256=deadbeef")
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
	if fired {
		t.Error("handler fired on invalid signature")
	}

	if _, err := rcv.Handle([]byte(samplePayload), ""); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("empty signature: want ErrInvalidSignature, got %v", err)
	}
}

func TestVerifyWebhook(t *testing.T) {
	body := []byte(samplePayload)
	if !VerifyWebhook(testSecret, body, sign(testSecret, samplePayload)) {
		t.Error("valid signature rejected")
	}
	if VerifyWebhook("other", body, sign(testSecret, samplePayload)) {
		t.Error("wrong secret accepted")
	}
}

func TestServeHTTP(t *testing.T) {
	rcv := NewWebhookReceiver(testSecret)
	var got *WebhookEvent
	rcv.On("message.received", func(e *WebhookEvent) { got = e })
	srv := httptest.NewServer(rcv)
	defer srv.Close()

	post := func(body, sig string) int {
		req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
		if sig != "" {
			req.Header.Set(SignatureHeader, sig)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := post(samplePayload, sign(testSecret, samplePayload)); code != http.StatusOK {
		t.Errorf("valid delivery status = %d", code)
	}
	if got == nil || got.ID != "evt_123" {
		t.Errorf("handler did not receive event via ServeHTTP: %+v", got)
	}
	if code := post(samplePayload, "sha256=bad"); code != http.StatusBadRequest {
		t.Errorf("invalid signature status = %d, want 400", code)
	}
}
