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
