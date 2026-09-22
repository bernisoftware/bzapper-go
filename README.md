# bZapper Go SDK

Official Go SDK for the [bZapper](https://bzapper.com.br) API — a multi-tenant
WhatsApp gateway. Connect numbers, send every message type, manage instances and
API keys, and track usage. Built on the standard library only (`net/http` +
`encoding/json`) — **zero external dependencies**.

## Install

```sh
go get github.com/bernisoftware/bzapper-go@v0.7.1
```

**Pin the exact version** (`@vX.Y.Z` in `go get`, which `go.mod` then records as
`require github.com/bernisoftware/bzapper-go v0.7.1`): every release note states
whether it changes the public surface (breaking vs additive), so upgrading is a
deliberate decision. Zero runtime dependencies — standard library only.

```go
import bzapper "github.com/bernisoftware/bzapper-go"
```

## Hello world

```go
package main

import (
	"context"
	"fmt"
	"log"

	bzapper "github.com/bernisoftware/bzapper-go"
)

func main() {
	client := bzapper.NewClient("bz_live_...")

	msg, err := client.SendText(context.Background(), bzapper.SendTextParams{
		SendBase: bzapper.SendBase{To: "+5511999999999"},
		Body:     "Olá do bZapper!",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("queued:", msg.MessageID)
}
```

The base URL defaults to `https://api.bzapper.com.br`. For dev/self-host, override it
with an option: `bzapper.NewClient("bz_live_...", bzapper.WithBaseURL("http://localhost:8080"))`.

## Configuration

`NewClient(apiKey string, opts ...Option)` returns a `*Client` that is safe
for concurrent use and makes no network call. (The older
`New(baseURL, apiKey string, opts ...Option)` still exists for backward compat.)

### Authentication

Create the API key in the bZapper panel → **API keys** (`bz_live_...`). A key
belongs to a **project** (numbers, inbox, contacts and stats are isolated per
project); to act on another project with an account-wide key, pass
`WithProjectID`. Never ship the key to a browser. An empty key makes every call
fail with `ErrInvalidArgument` before any request.

Every request sends `Authorization: Bearer <apiKey>`, `Accept: application/json`,
`X-Bzapper-Client: bzapper-go/<Version>` (also as `User-Agent`), a per-call
`X-Request-Id`, an `Idempotency-Key` on writes, `Content-Type: application/json`
when there is a body, and — when configured — `Accept-Language` / `X-Project-Id`.

| Option                       | Purpose                                              |
| ---------------------------- | ---------------------------------------------------- |
| `WithBaseURL(url)`           | Override the base URL (dev/self-host).               |
| `WithLocale("pt-BR")`        | Sets `Accept-Language` (localizes error messages).   |
| `WithTimeout(30*time.Second)`| Timeout per attempt (default 30s).                   |
| `WithMaxRetries(2)`          | Retries after the first attempt (default 2; 0 disables). |
| `WithProjectID(id)`          | Sends `X-Project-Id` (project scope).                |
| `WithHTTPClient(hc)`         | Supply your own `*http.Client` (proxy, transport…).  |

```go
client := bzapper.NewClient("bz_live_...",
	bzapper.WithLocale("pt-BR"),
	bzapper.WithTimeout(15*time.Second),
)
```

Every method takes a `context.Context` as its first argument.

## Common send fields (`SendBase`)

All message methods embed `SendBase`:

| Field             | Notes                                                       |
| ----------------- | ----------------------------------------------------------- |
| `To`              | Required. Destination in E.164 (`+5511...`) or a JID.       |
| `InstanceID`      | **Optional.** Force a specific number (bypasses rotation). Omit to auto-pick (rotation). |
| `PoolID`          | Rotate within a pool (when `InstanceID` is empty).          |
| `QuotedMessageID` | `wa_message_id` to reply to.                                |
| `QuotedParticipant` | Author (phone or JID) of the quoted/reacted message. Only needed in groups when that message isn't in bZapper history. |
| `ClientReference` | Echoed back in status events for correlation.               |
| `Mentions`        | Mentioned people (group messages): JIDs or plain phones (`5511...`, `+55 11 9...`). |
| `ScheduledAt`     | Future RFC 3339 time: the send is scheduled (`Status == "scheduled"`, see `ScheduledID`). |
| `Groups` / `Tags` | Contact-group / tag keys: sends 1:1 to every ACTIVE contact in them. |
| `Force`           | Skips the opt-out/suppression guard (transactional messages only). |
| `IdempotencyKey`  | Sent as the `Idempotency-Key` header (≤255 chars) instead of the per-call key the SDK generates. Retrying with the same key within 24h returns the same response without sending twice (`409 idempotency_in_progress`, `422 idempotency_key_reused`). |

## Messages — one example of each type

```go
ctx := context.Background()
to := bzapper.SendBase{To: "+5511999999999"}

// Text
client.SendText(ctx, bzapper.SendTextParams{SendBase: to, Body: "Olá!"})

// Image (use URL or Base64, never both)
client.SendImage(ctx, bzapper.SendMediaParams{SendBase: to,
	Media: bzapper.MediaInput{URL: "https://example.com/cat.png", Caption: "gatinho"}})

// Video
client.SendVideo(ctx, bzapper.SendMediaParams{SendBase: to,
	Media: bzapper.MediaInput{URL: "https://example.com/clip.mp4"}})

// Document
client.SendDocument(ctx, bzapper.SendMediaParams{SendBase: to,
	Media: bzapper.MediaInput{URL: "https://example.com/invoice.pdf", Filename: "invoice.pdf"}})

// Audio — set PTT for a voice note
client.SendAudio(ctx, bzapper.SendMediaParams{SendBase: to,
	Media: bzapper.MediaInput{URL: "https://example.com/voice.ogg", PTT: true}})

// Sticker
client.SendSticker(ctx, bzapper.SendMediaParams{SendBase: to,
	Media: bzapper.MediaInput{URL: "https://example.com/sticker.webp"}})

// Location
client.SendLocation(ctx, bzapper.SendLocationParams{SendBase: to,
	Latitude: -23.5505, Longitude: -46.6333, Name: "São Paulo"})

// Contact
client.SendContact(ctx, bzapper.SendContactParams{SendBase: to,
	ContactName: "Suporte bZapper"})

// Poll
client.SendPoll(ctx, bzapper.SendPollParams{SendBase: to,
	Name: "Canal favorito?", Options: []string{"WhatsApp", "Email"}, SelectableCount: 1})

// Reaction — QuotedMessageID + Emoji required
client.SendReaction(ctx, bzapper.SendReactionParams{
	SendBase: bzapper.SendBase{To: "+5511999999999", QuotedMessageID: "ABCD1234"},
	Emoji:    "👍"})

// Buttons (see caveat below)
client.SendButtons(ctx, bzapper.SendButtonsParams{SendBase: to, Body: "Escolha:",
	Buttons: []bzapper.Button{{ID: "yes", Title: "Sim"}, {ID: "no", Title: "Não"}}})

// List (see caveat below)
client.SendList(ctx, bzapper.SendListParams{SendBase: to, Body: "Cardápio:", ButtonText: "Ver",
	Sections: []bzapper.ListSection{{Title: "Bebidas",
		Rows: []bzapper.ListRow{{ID: "coffee", Title: "Café", Description: "Quentinho"}}}}})
```

> **Caveat (buttons & lists):** WhatsApp does not reliably render interactive
> buttons/lists (worse in groups). The API **always** sends an equivalent
> **numbered text menu** as a fallback, so recipients may see a plain text menu
> instead of native buttons.

All send methods return `(*bzapper.Message, error)` — the queued envelope with
`MessageID`, `Status` (`"queued"`) and the echoed `ClientReference`.

## Instances

```go
list, _ := client.ListInstances(ctx)
inst, _ := client.CreateInstance(ctx, bzapper.CreateInstanceParams{Phone: "+5511999999999", Nickname: "vendas"})
inst, _ = client.GetInstance(ctx, inst.ID)

// Connect by QR or pairing code
res, _ := client.ConnectInstance(ctx, inst.ID, bzapper.ConnectQR)   // res.QRCode
res, _ = client.ConnectInstance(ctx, inst.ID, bzapper.ConnectCode)  // res.PairCode

client.DisconnectInstance(ctx, inst.ID)
client.LogoutInstance(ctx, inst.ID)       // unpair (scan again to reconnect)
client.ClearInstanceSession(ctx, inst.ID) // wipe a stuck pairing
client.ArchiveInstance(ctx, inst.ID)      // hide; ListInstances(ctx, bzapper.ListInstancesParams{Archived: "1"})
client.UnarchiveInstance(ctx, inst.ID)
client.DeleteInstance(ctx, inst.ID)

// Proxy, privacy and inbound filters
client.SetInstanceProxy(ctx, inst.ID, "http://user:pass@proxy.example:8080")
client.SetPrivacy(ctx, inst.ID, bzapper.PrivacyParams{Setting: "last", Value: "contacts"})
yes := true
client.SetInboundFilters(ctx, inst.ID, bzapper.InboundFilters{IgnoreGroups: &yes})

// Official rail (WhatsApp Cloud API) — projects created with APIMode "OFFICIAL"
acct, _ := client.GetOfficialAccount(ctx) // acct.Status, acct.QualityRating
```

## Message management and scheduled sends

```go
ref, _ := client.EditMessage(ctx, msgID, "texto corrigido")
client.RevokeMessage(ctx, msgID, true) // delete for everyone
client.ForwardMessage(ctx, bzapper.ForwardMessageParams{
	InstanceID: inst.ID, To: "+5511988887777", FromChat: "5511977776666@s.whatsapp.net", WAMessageID: ref.WAMessageID,
})
client.MarkRead(ctx, msgID, bzapper.MarkReadParams{InstanceID: inst.ID, Chat: "5511977776666@s.whatsapp.net"})

pending, _ := client.ListScheduledWithParams(ctx, bzapper.ListScheduledParams{Limit: 50})
client.CancelScheduled(ctx, pending[0].ID)
```

## API keys

```go
keys, _ := client.ListKeys(ctx)

created, _ := client.CreateKey(ctx, bzapper.CreateKeyParams{Name: "ci", Role: bzapper.RoleAgent})
// created.APIKey is the RAW key — shown once, store it now.

client.RevokeKey(ctx, created.Key.ID)
```

## Usage

```go
usage, _ := client.GetUsage(ctx, bzapper.GetUsageParams{
	From: "2026-06-01T00:00:00Z",
	To:   "2026-06-30T23:59:59Z",
})
fmt.Println(usage.Total, usage.DeliveryRate)
```

## Webhooks

Two parts: **manage** subscriptions with the client, and **receive** deliveries
with `WebhookReceiver`.

### Manage subscriptions

```go
created, _ := client.CreateWebhook(ctx, bzapper.CreateWebhookParams{
	URL:        "https://example.com/webhooks",
	EventTypes: []string{"message.received", "message.failed"}, // empty = all
})
// created.Secret is shown ONLY ONCE — store it; you pass it to the receiver.
fmt.Println(created.ID, created.Secret)

client.ListWebhooks(ctx)
client.TestWebhook(ctx, created.ID, "message.received")
client.WebhookDeliveries(ctx, created.ID, 20)
client.TriggerWebhookEvent(ctx, "message.received") // sample event to every matching webhook

active := false
client.UpdateWebhook(ctx, created.ID, bzapper.UpdateWebhookParams{Active: &active})
client.DeleteWebhook(ctx, created.ID)
```

`UpdateWebhookParams` uses pointers so you only send what you set. Use
`Secret: ptr("regenerate")` to rotate the signing secret.

### Receive deliveries

Each delivery is signed: `X-Bzapper-Signature: sha256=<hex>` where the hex is
`HMAC-SHA256(secret, raw_body)`. The receiver verifies the signature (timing-safe,
against the **raw** body), parses the envelope into a typed `*WebhookEvent`, and
routes it to your handlers. A `*WebhookReceiver` is itself an `http.Handler`:

```go
secret := os.Getenv("BZAPPER_WEBHOOK_SECRET") // the created.Secret from above

http.Handle("/webhooks", bzapper.NewWebhookReceiver(secret).
	On("message.received", func(e *bzapper.WebhookEvent) {
		fmt.Println(e.Sender.Name, e.Payload["body"])
	}).
	On("message.failed", func(e *bzapper.WebhookEvent) {
		fmt.Println("failed:", e.ID)
	}).
	OnAny(func(e *bzapper.WebhookEvent) {
		// runs for every event — e.g. store e.ID for idempotency.
	}))

log.Fatal(http.ListenAndServe(":8080", nil))
```

`WebhookEvent` carries `ID`, `Type`, `Timestamp`, `InstanceID`, `ClientReference`,
`Group`, `Sender`, `Mentions`, `Payload` and the original `Raw` bytes. The API may
retry deliveries, so dedupe on `e.ID`.

For non-`net/http` frameworks, drive it directly with the raw body and signature
header:

```go
rcv := bzapper.NewWebhookReceiver(secret).On("message.received", handler)

event, err := rcv.Handle(rawBody, signatureHeader)
if errors.Is(err, bzapper.ErrInvalidSignature) {
	// reject: do NOT process — return 400
}
```

You can also verify/parse without a receiver via `bzapper.VerifyWebhook(secret,
body, sig)` and `bzapper.ConstructWebhookEvent(secret, body, sig)`.

## Groups, presence and conversations

`instance_id` is sent in the query for these endpoints (or in the body where the
API requires it); `jid` is always a path segment.

```go
// Presence — works in groups too! Pass a group JID as To.
client.PresenceChat(ctx, bzapper.PresenceChatParams{
	InstanceID: inst.ID,
	To:         "120363021234567890@g.us", // group JID
	State:      bzapper.PresenceTyping,
})

// Conversations
convs, _ := client.ListConversations(ctx, inst.ID)
hist, _ := client.ConversationHistory(ctx, "120363021234567890@g.us",
	bzapper.ConversationHistoryParams{InstanceID: inst.ID, Limit: 50})

// Chat flags (archive / pin / mark read|unread)
client.ArchiveChat(ctx, "5511999999999@s.whatsapp.net", inst.ID, true)
client.PinChat(ctx, "5511999999999@s.whatsapp.net", inst.ID, true)
client.MarkChat(ctx, "5511999999999@s.whatsapp.net", inst.ID, true)
client.MuteChat(ctx, "5511999999999@s.whatsapp.net", inst.ID, true)

// Labels (WhatsApp Business)
label, _ := client.CreateLabel(ctx, bzapper.CreateLabelParams{InstanceID: inst.ID, Name: "VIP"})
client.ApplyChatLabel(ctx, "5511999999999@s.whatsapp.net", bzapper.ApplyChatLabelParams{
	InstanceID: inst.ID, LabelID: label.ID, Apply: true,
})

// Block and calls
client.BlockContact(ctx, "5511999999999@s.whatsapp.net", inst.ID)
blocked, _ := client.GetBlocklist(ctx, inst.ID) // blocked.Data
client.RejectCall(ctx, bzapper.RejectCallParams{InstanceID: inst.ID, CallFrom: "5511...@s.whatsapp.net", CallID: "..."})

// Groups
groups, _ := client.ListGroups(ctx, inst.ID)
group, _ := client.CreateGroup(ctx, inst.ID, bzapper.CreateGroupParams{
	Name:         "Equipe",
	Participants: []string{"+5511988887777", "+5511977776666"},
})
group, _ = client.GetGroup(ctx, group.JID, inst.ID)

client.UpdateGroupParticipants(ctx, group.JID, inst.ID, bzapper.UpdateGroupParticipantsParams{
	Action:       bzapper.GroupPromote,
	Participants: []string{"+5511988887777"},
})

topic := "Avisos da equipe"
client.UpdateGroup(ctx, group.JID, inst.ID, bzapper.UpdateGroupParams{Topic: &topic})
invite, _ := client.GroupInviteLink(ctx, group.JID, bzapper.GroupInviteLinkParams{InstanceID: inst.ID}) // invite.InviteLink
reqs, _ := client.ListJoinRequests(ctx, group.JID, inst.ID)
client.UpdateJoinRequests(ctx, group.JID, inst.ID, bzapper.UpdateJoinRequestsParams{
	Participants: []string{reqs.Data[0].JID}, Approve: true,
})
preview, _ := client.PreviewGroupInvite(ctx, inst.ID, bzapper.JoinGroupParams{Code: "AbCdEf123"}) // name/size WITHOUT joining
client.JoinGroup(ctx, inst.ID, bzapper.JoinGroupParams{Code: "AbCdEf123"})
client.LeaveGroup(ctx, group.JID, inst.ID)
```

## Contacts and profile

```go
// Which phones are on WhatsApp?
res, _ := client.ContactsCheck(ctx, bzapper.ContactsCheckParams{
	InstanceID: inst.ID,
	Phones:     []string{"+5511988887777", "+5511977776666"},
})
for _, c := range res.Data {
	fmt.Println(c.Phone, c.IsRegistered, c.JID)
}

// Update the instance's WhatsApp profile (unset fields stay unchanged).
name := "Suporte bZapper"
client.SetProfile(ctx, inst.ID, bzapper.SetProfileParams{DisplayName: &name})

// CRM: the project's contact base. The contact↔project/number link is
// maintained automatically by the API — filters only read it.
c, _ := client.CreateContact(ctx, bzapper.CreateContactParams{Phone: "+5511988887777", Name: "Ana"})
client.MutateContactTags(ctx, c.ID, bzapper.TaxonMutation{Add: []string{"vip"}})
client.MutateContactGroups(ctx, c.ID, bzapper.TaxonMutation{Add: []string{"clientes"}})
client.AddContactNote(ctx, c.ID, "Pediu retorno amanhã")
page, _ := client.ListContacts(ctx, bzapper.ListContactsParams{Tags: "vip", Status: "active", Limit: 50})
hist, _ := client.GetContactHistory(ctx, c.ID, bzapper.ContactHistoryParams{Limit: 20})
client.OptOutContact(ctx, c.ID) // or OptInContact / SuppressContact

client.CreateTag(ctx, bzapper.CreateTaxonParams{Key: "vip", Name: "VIP", Color: "#22c55e"})
client.CreateContactGroup(ctx, bzapper.CreateTaxonParams{Key: "clientes", Name: "Clientes"})
client.CreateSuppression(ctx, bzapper.CreateSuppressionParams{Phone: "+5511999998888", Reason: "pediu para sair"})
client.DeleteSuppression(ctx, "+5511999998888")
```

## Campaigns and pools

```go
pool, _ := client.CreatePool(ctx, bzapper.CreatePoolParams{Name: "vendas", Strategy: bzapper.PoolHealthWeighted})
client.AddPoolNumber(ctx, pool.ID, inst.ID)
elig, _ := client.GetCampaignEligibility(ctx, bzapper.CampaignEligibilityParams{PoolID: pool.ID})

logo, _ := bzapper.FileFromPath("promo.png")
media, _ := client.UploadCampaignMedia(ctx, logo) // media.URL

camp, _ := client.CreateCampaign(ctx, bzapper.CampaignCreateParams{
	Name: "Black Friday", PoolID: pool.ID, PacingProfile: "conservative",
	Variations: []bzapper.CampaignVariation{
		{Body: "Oi {nome}! {Oferta|Promo} só hoje", Media: map[string]any{"url": media.URL}},
	},
})
client.AddCampaignRecipients(ctx, camp.ID, bzapper.CampaignRecipientsParams{
	ContactFilter: &bzapper.ContactFilter{Tags: []string{"vip"}},
})
dry, _ := client.DryRunCampaign(ctx, camp.ID) // missing variables, warnings, ETA
started, _ := client.StartCampaignWithResult(ctx, camp.ID) // started.Waiting = held by the send window
client.PauseCampaign(ctx, camp.ID)
recipients, _ := client.ListCampaignRecipientsWithParams(ctx, camp.ID, bzapper.ListCampaignRecipientsParams{Limit: 100})
```

## Account, projects, users, brand and billing

```go
me, _ := client.GetMe(ctx) // account, user, role, scopes
proj, _ := client.CreateProjectWithParams(ctx, bzapper.CreateProjectParams{Name: "Loja", APIMode: "UNOFFICIAL"})
health, _ := client.GetProjectsHealth(ctx) // numbers per status, per project
client.SetProjectBrand(ctx, proj.ID, bzapper.BrandProfile{About: "Atendimento 8h–18h"})
file, _ := bzapper.FileFromPath("logo.png")
client.UploadProjectLogo(ctx, proj.ID, file)

client.InviteUser(ctx, bzapper.InviteUserParams{Email: "ana@empresa.com", Role: bzapper.RoleAgent})

ent, _ := client.GetMyEntitlements(ctx) // plan, limits, usage
cart, _ := client.ChangeAddon(ctx, bzapper.ChangeAddonParams{Kind: bzapper.AddonNumber, Delta: 1})
pay, _ := client.CheckoutAddonCart(ctx, bzapper.CheckoutAddonCartParams{}) // confirm pay.ClientSecret with Stripe.js
invoices, _ := client.ListMyInvoices(ctx)
```

## bZapper Connect (partners)

For **partner software** that lets its own customers subscribe to bZapper Pro and
connect WhatsApp without leaving the partner's product. The flow:

1. Your **backend** creates a session with the partner secret (`bz_partner_...`).
2. Your front-end opens the embedded component with the `session_token`.
3. When the customer finishes (Pro paid + number connected), the component emits a
   one-time `code` (valid 10 min). Your front-end sends it to your backend.
4. Your backend exchanges the `code` for the customer's API key (`bz_live_...`) and
   uses the regular `Client` with it.

The partner secret authenticates with the same `Authorization: Bearer` header as an
API key, but only on `/partner/*`. **Never** send it to a browser.

```go
partner := bzapper.NewPartnerClient(os.Getenv("BZAPPER_PARTNER_SECRET"))

// 1) Your front-end asks your backend to open Connect for the logged-in customer.
http.HandleFunc("/bzapper/session", func(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r) // your own auth
	s, err := partner.CreateConnectSession(r.Context(), bzapper.CreateConnectSessionParams{
		ExternalID: u.CustomerID, // YOUR id — same id = same connection
		Customer: bzapper.ConnectCustomer{
			Name:    u.Name,
			Email:   u.Email,        // required (plus Name or Company)
			Phone:   u.Phone,        // optional, E.164 — pre-fills the number
			Company: u.CompanyName,  // optional — becomes the account/project name
			Country: "BR",           // optional, ISO-3166 alpha-2 — sets the currency
		},
		Locale: "pt-BR",
	})
	if err != nil {
		http.Error(w, "connect unavailable", http.StatusBadGateway)
		return
	}
	// 2) Only the short-lived (30 min) session token goes to the browser.
	json.NewEncoder(w).Encode(map[string]string{"session": s.SessionToken})
})

// 3) The component finished: exchange the one-time code for the customer's key.
http.HandleFunc("/bzapper/exchange", func(w http.ResponseWriter, r *http.Request) {
	var in struct{ Code string `json:"code"` }
	json.NewDecoder(r.Body).Decode(&in)

	conn, err := partner.ExchangeCode(r.Context(), in.Code)
	if err != nil {
		var apiErr *bzapper.Error
		if errors.As(err, &apiErr) && apiErr.Code == "invalid_code" {
			http.Error(w, "code expired or already used", http.StatusBadRequest)
			return
		}
		http.Error(w, "exchange failed", http.StatusBadGateway)
		return
	}
	// conn.APIKey is shown ONLY ONCE — store it (encrypted) against conn.ExternalID.
	// Lost it? partner.RotateConnectionKey(ctx, conn.ID) issues a new one.
	saveKey(conn.ExternalID, conn.ID, conn.APIKey)
	w.WriteHeader(http.StatusNoContent)
})

// 4) From now on, act on the customer's WhatsApp with the regular client.
client := bzapper.NewClient(loadKey("cust-42"))
_, err := client.SendText(ctx, bzapper.SendTextParams{
	SendBase: bzapper.SendBase{To: "+5511999999999"},
	Body:     "Olá!",
})
var apiErr *bzapper.Error
if errors.As(err, &apiErr) {
	switch apiErr.Code {
	case bzapper.ErrCodeConnectSuspended: // HTTP 402
		// The customer's Pro is unpaid. Do not retry in a loop: the key resumes
		// by itself once paid (you receive connect.resumed).
	case bzapper.ErrCodeConnectRevoked: // HTTP 401
		// The connection was ended — drop the key and offer to connect again.
	}
}
```

The key is scoped to the customer's project: it operates numbers and messages, but
cannot touch billing, users, keys or webhooks of the account.

### Managing connections

```go
me, _ := partner.Me(ctx) // who the secret belongs to

list, _ := partner.ListConnections(ctx, bzapper.ListConnectionsParams{
	ExternalID: "cust-42",                  // optional
	Status:     bzapper.ConnectionSuspended, // optional
})
conn, _ := partner.GetConnection(ctx, list[0].ID) // status, account, project, numbers

rotated, _ := partner.RotateConnectionKey(ctx, conn.ID) // previous key stops working
fmt.Println(rotated.APIKey)                              // 409 connection_not_active if not completed/revoked

partner.RevokeConnection(ctx, conn.ID) // 204; revokes the key, does NOT cancel the customer's plan
```

Connection status (`bzapper.ConnectionStatus`): `ConnectionPendingAccount`,
`ConnectionPendingPayment`, `ConnectionPendingNumber`, `ConnectionActive`,
`ConnectionSuspended` (key answers 402 `connect_suspended`), `ConnectionRevoked`.

### Partner webhooks

Deliveries to the partner's webhook use the **same** signature scheme
(`X-Bzapper-Signature: sha256=<hex>` over the raw body), so `WebhookReceiver`,
`VerifyWebhook` and `ConstructWebhookEvent` work unchanged. The envelope carries an
extra `connection` block, exposed as `e.Connection` (nil on regular deliveries),
telling you which of your customers the event is about. Besides the lifecycle events
below, you also receive the regular project events (`message.*`, `instance.*`…) of
every active connection — except QR/pairing codes.

```go
secret := os.Getenv("BZAPPER_PARTNER_WEBHOOK_SECRET")

http.Handle("/webhooks/bzapper", bzapper.NewWebhookReceiver(secret).
	On(bzapper.EventConnectCompleted, func(e *bzapper.WebhookEvent) {
		markConnected(e.Connection.ExternalID, e.Connection.ID)
	}).
	On(bzapper.EventConnectSuspended, func(e *bzapper.WebhookEvent) {
		pauseWhatsApp(e.Connection.ExternalID) // key now answers 402 connect_suspended
	}).
	On(bzapper.EventConnectResumed, func(e *bzapper.WebhookEvent) {
		resumeWhatsApp(e.Connection.ExternalID)
	}).
	On(bzapper.EventConnectRevoked, func(e *bzapper.WebhookEvent) {
		deleteKey(e.Connection.ExternalID) // the key never works again
	}).
	On("message.received", func(e *bzapper.WebhookEvent) {
		routeInbound(e.Connection.ExternalID, e)
	}))
```

`e.Connection` has `ID`, `ExternalID`, `AccountID`, `ProjectID` and `Status`. Dedupe
on `e.ID`, as with any webhook.

### Customer side: connected apps

With the customer's own API key, list and disconnect partner apps using the account:

```go
apps, _ := client.ListConnectedApps(ctx) // []PartnerConnection, with PartnerName / PartnerLogoURL
client.RevokeConnectedApp(ctx, apps[0].ID) // admin; the partner's key stops working immediately
```

## Errors, retries and idempotency

Every API failure is a `*bzapper.Error` — non-2xx responses, a 2xx whose body
is not JSON (`Code == "INVALID_RESPONSE"`) and network failures/timeouts
(`Code == "NETWORK_ERROR"`, `StatusCode == 0`). **Branch on the stable, neutral
`Code`** — never parse the localized `Message`. Send `RequestID` to support: it
matches the API logs.

| Field | Meaning |
|---|---|
| `Code` | stable code (`body.code`, else `body.error`, else `HTTP_<status>`) |
| `Message` / `Locale` | localized text (humans only) |
| `StatusCode` | HTTP status (0 on network errors) |
| `Type` | `authentication` (401), `permission_denied` (403), `not_found` (404), `conflict` (409), `validation` (400/422), `rate_limit` (429), `server` (5xx), `network`, `api` (anything else) |
| `RequestID` | response `X-Request-Id`, else the one the SDK sent |
| `RetryAfter` | wait asked by a 429 (`Retry-After`) |
| `RequiredScope` | scope the key lacks (403, `X-Required-Scope`) |
| `Body` | decoded error body (structured detail) |

The typed classes of the other SDKs are sentinels for `errors.Is` in Go:
`ErrAuthentication`, `ErrPermissionDenied`, `ErrNotFound`, `ErrConflict`,
`ErrValidation`, `ErrRateLimit`, `ErrServer`, `ErrNetwork`. Argument errors
detected before any request (empty API key, a path parameter that is empty,
`"."` or `".."`) match `ErrInvalidArgument` and are not `*Error`.

```go
_, err := client.GetInstance(ctx, id)
switch {
case errors.Is(err, bzapper.ErrNotFound):
	// …
case errors.Is(err, bzapper.ErrRateLimit):
	var e *bzapper.Error
	errors.As(err, &e)
	log.Printf("slow down for %s (request %s)", e.RetryAfter, e.RequestID)
}
```

**Retries.** Network errors/timeouts and `429`, `502`, `503`, `504` are retried
automatically (`WithMaxRetries`, default 2), waiting `Retry-After` when present
(capped at 60s) or an exponential backoff (0.5s, 1s, 2s… up to 8s, +25% jitter).
A `500` or any `4xx` returns at once. Cancel the `ctx` to stop waiting.

**Idempotency.** Every write (POST/PUT/PATCH/DELETE) carries an
`Idempotency-Key` generated per call and **repeated on every retry** — together
with the repeated `X-Request-Id`, that is what makes retrying a send safe (the
API answers a repeat with the original response and `Idempotent-Replayed: true`).
To use your own key (e.g. your order id), set `SendBase.IdempotencyKey` on the
message sends, or for any write:

```go
ctx := bzapper.ContextWithIdempotencyKey(ctx, "order-4471")
_, err := client.CreateContact(ctx, bzapper.CreateContactParams{Phone: "+5511988887777"})
```

```go
_, err := client.SendText(ctx, bzapper.SendTextParams{SendBase: to, Body: "hi"})
if err != nil {
	var apiErr *bzapper.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "not_connected":
			// re-scan the QR
		case "rate_limited":
			// back off and retry (HTTP 429)
		default:
			log.Printf("api error %s (http %d): %s", apiErr.Code, apiErr.StatusCode, apiErr.Message)
		}
		return
	}
	log.Fatalf("transport error: %v", err) // network/timeout
}
```

Buttons and lists may be rendered by WhatsApp as a **numbered text menu**
fallback — design the text so it still reads well as a menu.

## Example program

A runnable example lives in [`examples/main.go`](examples/main.go):

```sh
BZAPPER_BASE_URL=http://localhost:8080 BZAPPER_API_KEY=bz_live_... go run ./examples
```

## License

MIT © Berni Software. See [LICENSE](LICENSE).
