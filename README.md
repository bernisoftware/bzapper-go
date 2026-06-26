# bZapper Go SDK

Official Go SDK for the [bZapper](https://bzapper.com.br) API — a multi-tenant
WhatsApp gateway. Connect numbers, send every message type, manage instances and
API keys, and track usage. Built on the standard library only (`net/http` +
`encoding/json`) — **zero external dependencies**.

## Install

```sh
go get github.com/bernisoftware/bzapper-go
```

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
	client := bzapper.New("http://localhost:8080", "bz_live_...")

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

## Configuration

`New(baseURL, apiKey string, opts ...Option)` returns a `*Client` that is safe
for concurrent use. Every request sends `Authorization: Bearer <apiKey>`,
`Content-Type: application/json` and (when set) `Accept-Language: <locale>`.

| Option                       | Purpose                                              |
| ---------------------------- | ---------------------------------------------------- |
| `WithLocale("pt-BR")`        | Sets `Accept-Language` (localizes error messages).   |
| `WithTimeout(30*time.Second)`| Request timeout (default 30s).                       |
| `WithHTTPClient(hc)`         | Supply your own `*http.Client` (proxy, transport…).  |

```go
client := bzapper.New("https://api.bzapper.com.br", "bz_live_...",
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
| `InstanceID`      | Pin a specific number (bypasses rotation).                  |
| `PoolID`          | Rotate within a pool (when `InstanceID` is empty).          |
| `QuotedMessageID` | `wa_message_id` to reply to.                                |
| `ClientReference` | Echoed back in status events for correlation.               |
| `Mentions`        | JIDs mentioned (group messages).                            |

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

invite, _ := client.GroupInvite(ctx, group.JID, inst.ID) // invite.URL / invite.Code
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
```

## Error handling

Non-2xx responses return a typed `*bzapper.Error`. Always branch on the stable,
neutral `Code` — **never** parse the localized `Message`.

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

`Error` fields: `Code` (stable), `Message` (localized, human-only), `Locale`,
`StatusCode`.

## Example program

A runnable example lives in [`examples/main.go`](examples/main.go):

```sh
BZAPPER_BASE_URL=http://localhost:8080 BZAPPER_API_KEY=bz_live_... go run ./examples
```

## License

MIT © Berni Software. See [LICENSE](LICENSE).
