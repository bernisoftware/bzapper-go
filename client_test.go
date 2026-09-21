package bzapper

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL, "bz_live_test", WithLocale("pt-BR"))
}

func TestSendText_SetsHeadersAndBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer bz_live_test" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Accept-Language"); got != "pt-BR" {
			t.Errorf("Accept-Language = %q", got)
		}
		if r.URL.Path != "/messages/text" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if want := `"body":"hi"`; !contains(string(body), want) {
			t.Errorf("body %q missing %q", body, want)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message_id":"m1","status":"queued"}`))
	})

	msg, err := c.SendText(context.Background(), SendTextParams{
		SendBase: SendBase{To: "+5511999999999"},
		Body:     "hi",
	})
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if msg.MessageID != "m1" || msg.Status != "queued" {
		t.Errorf("got %+v", msg)
	}
}

func TestErrorResponse_TypedError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"not_connected","message":"Número desconectado.","locale":"pt-BR"}`))
	})

	_, err := c.SendText(context.Background(), SendTextParams{SendBase: SendBase{To: "x"}, Body: "y"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Code != "not_connected" || apiErr.StatusCode != http.StatusConflict {
		t.Errorf("got %+v", apiErr)
	}
}

func TestConnectInstance_Method(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("method"); got != "code" {
			t.Errorf("method = %q", got)
		}
		_, _ = w.Write([]byte(`{"status":"code_pending","pair_code":"ABCD1234"}`))
	})

	res, err := c.ConnectInstance(context.Background(), "id-1", ConnectCode)
	if err != nil {
		t.Fatalf("ConnectInstance: %v", err)
	}
	if res.PairCode != "ABCD1234" || res.Status != StatusCodePending {
		t.Errorf("got %+v", res)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSendText_IdempotencyKeyHeader(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Idempotency-Key"); got != "order-42" {
			t.Errorf("Idempotency-Key = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if contains(string(body), "order-42") || contains(string(body), "idempotency") {
			t.Errorf("idempotency key leaked into body: %s", body)
		}
		if want := `"quoted_participant":"+5511988887777"`; !contains(string(body), want) {
			t.Errorf("body %q missing %q", body, want)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message_id":"m1","status":"queued"}`))
	})
	_, err := c.SendText(context.Background(), SendTextParams{
		SendBase: SendBase{To: "x", QuotedMessageID: "q1", QuotedParticipant: "+5511988887777", IdempotencyKey: "order-42"},
		Body:     "hi",
	})
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
}

func TestSendText_NoIdempotencyKeyByDefault(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Idempotency-Key"]; ok {
			t.Errorf("unexpected Idempotency-Key header")
		}
		_, _ = w.Write([]byte(`{"message_id":"m1","status":"queued"}`))
	})
	if _, err := c.SendText(context.Background(), SendTextParams{SendBase: SendBase{To: "x"}, Body: "y"}); err != nil {
		t.Fatalf("SendText: %v", err)
	}
}

func TestPreviewGroupInvite(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/groups/join/preview" || r.URL.Query().Get("instance_id") != "i1" {
			t.Errorf("got %s %s", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`{"jid":"1@g.us","name":"G","size":3,"participants":[{"jid":"9@lid","phone":"+5511999999999","lid":"9@lid","is_admin":true,"is_super_admin":false}]}`))
	})
	g, err := c.PreviewGroupInvite(context.Background(), "i1", JoinGroupParams{Code: "ABC"})
	if err != nil {
		t.Fatalf("PreviewGroupInvite: %v", err)
	}
	if g.Size != 3 || g.Participants[0].Phone != "+5511999999999" || g.Participants[0].LID != "9@lid" {
		t.Errorf("got %+v", g)
	}
}
