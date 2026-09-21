package bzapper

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestPartnerClient(t *testing.T, handler http.HandlerFunc) *PartnerClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewPartnerClient("bz_partner_test", WithBaseURL(srv.URL), WithLocale("pt-BR"))
}

const connectionJSON = `{
	"id": "conn_1", "external_id": "cust-42", "status": "active",
	"account_id": "acc_1", "project_id": "proj_1",
	"customer": {"name": "Ana Souza", "email": "ana@boxy.com", "company": "Boxy Pharma"},
	"numbers": [{"id": "inst_1", "phone": "+5511988887777", "status": "connected"}],
	"activated_at": "2026-09-17T12:00:00Z", "suspended_at": null, "revoked_at": null,
	"created_at": "2026-09-17T11:00:00Z"
}`

func TestCreateConnectSession_SetsAuthAndBody(t *testing.T) {
	p := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/partner/connect-sessions" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer bz_partner_test" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Bzapper-Client"); got != ClientID {
			t.Errorf("X-Bzapper-Client = %q", got)
		}
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("body: %v", err)
		}
		if body["external_id"] != "cust-42" || body["locale"] != "pt-BR" {
			t.Errorf("body = %s", raw)
		}
		cust, _ := body["customer"].(map[string]any)
		if cust["email"] != "ana@boxy.com" || cust["name"] != "Ana Souza" {
			t.Errorf("customer = %v", cust)
		}
		if _, ok := cust["phone"]; ok {
			t.Errorf("empty phone should be omitted: %s", raw)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"session_token":"cs_abc","expires_at":"2026-09-17T12:30:00Z","connection":` + connectionJSON + `}`))
	})

	s, err := p.CreateConnectSession(context.Background(), CreateConnectSessionParams{
		ExternalID: "cust-42",
		Customer:   ConnectCustomer{Name: "Ana Souza", Email: "ana@boxy.com"},
		Locale:     "pt-BR",
	})
	if err != nil {
		t.Fatalf("CreateConnectSession: %v", err)
	}
	if s.SessionToken != "cs_abc" || s.Connection.ID != "conn_1" || s.Connection.Status != ConnectionActive {
		t.Errorf("got %+v", s)
	}
	if s.Connection.SuspendedAt != nil || s.Connection.ActivatedAt == nil {
		t.Errorf("nullable timestamps: %+v", s.Connection)
	}
	if len(s.Connection.Numbers) != 1 || s.Connection.Numbers[0].Phone != "+5511988887777" {
		t.Errorf("numbers: %+v", s.Connection.Numbers)
	}
}

func TestExchangeCode(t *testing.T) {
	p := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/partner/connect/exchange" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"code":"cc_91ab"}` {
			t.Errorf("body = %s", raw)
		}
		conn := connectionJSON[:len(connectionJSON)-1] + `, "api_key": "bz_live_xyz"}`
		_, _ = w.Write([]byte(conn))
	})

	res, err := p.ExchangeCode(context.Background(), "cc_91ab")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if res.APIKey != "bz_live_xyz" || res.ID != "conn_1" || res.ExternalID != "cust-42" {
		t.Errorf("got %+v", res)
	}
}

func TestListConnections_QueryFilter(t *testing.T) {
	p := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/partner/connections" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("external_id") != "cust-42" || q.Get("status") != "suspended" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"data":[` + connectionJSON + `]}`))
	})

	list, err := p.ListConnections(context.Background(), ListConnectionsParams{
		ExternalID: "cust-42", Status: ConnectionSuspended,
	})
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(list) != 1 || list[0].Customer.Company != "Boxy Pharma" {
		t.Errorf("got %+v", list)
	}
}

func TestListConnections_NoFilterSendsNoQuery(t *testing.T) {
	p := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	if _, err := p.ListConnections(context.Background(), ListConnectionsParams{}); err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
}

func TestRotateConnectionKey(t *testing.T) {
	p := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/partner/connections/conn_1/rotate-key" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"conn_1","status":"active","api_key":"bz_live_new"}`))
	})

	res, err := p.RotateConnectionKey(context.Background(), "conn_1")
	if err != nil {
		t.Fatalf("RotateConnectionKey: %v", err)
	}
	if res.APIKey != "bz_live_new" {
		t.Errorf("got %+v", res)
	}
}

func TestRevokeConnection_NoContent(t *testing.T) {
	p := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/partner/connections/conn_1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := p.RevokeConnection(context.Background(), "conn_1"); err != nil {
		t.Fatalf("RevokeConnection: %v", err)
	}
}

func TestPartnerMe(t *testing.T) {
	p := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/partner/me" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"p1","slug":"bfocus","name":"bFocus","key_scopes":["messages:send"]}`))
	})
	me, err := p.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.Slug != "bfocus" || len(me.KeyScopes) != 1 {
		t.Errorf("got %+v", me)
	}
}

func TestConnectedApps_CustomerSide(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer bz_live_test" {
			t.Errorf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/me/connections":
			_, _ = w.Write([]byte(`{"data":[{"id":"conn_1","status":"active","partner_name":"bFocus","partner_logo_url":"https://x/logo.png"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/me/connections/conn_1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	apps, err := c.ListConnectedApps(context.Background())
	if err != nil {
		t.Fatalf("ListConnectedApps: %v", err)
	}
	if len(apps) != 1 || apps[0].PartnerName != "bFocus" || apps[0].PartnerLogoURL == "" {
		t.Errorf("got %+v", apps)
	}
	if err := c.RevokeConnectedApp(context.Background(), "conn_1"); err != nil {
		t.Fatalf("RevokeConnectedApp: %v", err)
	}
}

func TestConnectSuspended_TypedError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"code":"connect_suspended","message":"Pro em atraso.","locale":"pt-BR"}`))
	})
	_, err := c.SendText(context.Background(), SendTextParams{SendBase: SendBase{To: "x"}, Body: "y"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeConnectSuspended || apiErr.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("got %v", err)
	}
}

const partnerPayload = `{
	"event_id": "evt_c1",
	"event_type": "connect.suspended",
	"timestamp": "2026-09-17T12:00:00Z",
	"payload": {"reason": "payment_failed"},
	"connection": {"id": "conn_1", "external_id": "cust-42", "account_id": "acc_1", "project_id": "proj_1", "status": "suspended"}
}`

func TestPartnerWebhook_ConnectionEnvelope(t *testing.T) {
	sig := sign(testSecret, partnerPayload)
	if !VerifyWebhook(testSecret, []byte(partnerPayload), sig) {
		t.Fatal("valid partner signature rejected")
	}

	var got *WebhookEvent
	rcv := NewWebhookReceiver(testSecret).On(EventConnectSuspended, func(e *WebhookEvent) { got = e })
	if _, err := rcv.Handle([]byte(partnerPayload), sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got == nil || got.Connection == nil {
		t.Fatal("handler did not receive connection")
	}
	cn := got.Connection
	if cn.ID != "conn_1" || cn.ExternalID != "cust-42" || cn.AccountID != "acc_1" ||
		cn.ProjectID != "proj_1" || cn.Status != ConnectionSuspended {
		t.Errorf("connection mapping: %+v", cn)
	}

	if _, err := ConstructWebhookEvent(testSecret, []byte(partnerPayload), "sha256=bad"); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("want ErrInvalidSignature, got %v", err)
	}

	// Regular (non-partner) envelopes keep Connection nil.
	ev, err := ConstructWebhookEvent(testSecret, []byte(samplePayload), sign(testSecret, samplePayload))
	if err != nil || ev.Connection != nil {
		t.Errorf("regular envelope: err=%v connection=%+v", err, ev.Connection)
	}
}
