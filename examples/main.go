// Command example demonstrates every bZapper Go SDK message type plus
// instance, key and usage operations. It does not run network calls unless you
// provide BZAPPER_BASE_URL and BZAPPER_API_KEY.
//
// Run:
//
//	BZAPPER_BASE_URL=http://localhost:8080 BZAPPER_API_KEY=bz_live_... go run ./examples
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	bzapper "github.com/bernisoftware/bzapper-go"
)

func main() {
	apiKey := os.Getenv("BZAPPER_API_KEY")
	if apiKey == "" {
		log.Println("set BZAPPER_API_KEY to run this example")
		return
	}

	opts := []bzapper.Option{
		bzapper.WithLocale("pt-BR"),
		bzapper.WithTimeout(30 * time.Second),
	}
	// BZAPPER_BASE_URL é opcional: sem ele, o SDK aponta para produção.
	if baseURL := os.Getenv("BZAPPER_BASE_URL"); baseURL != "" {
		opts = append(opts, bzapper.WithBaseURL(baseURL))
	}
	client := bzapper.NewClient(apiKey, opts...)

	ctx := context.Background()
	const to = "+5511999999999"

	// Text
	mustSend(client.SendText(ctx, bzapper.SendTextParams{
		SendBase: bzapper.SendBase{To: to, ClientReference: "demo-1"},
		Body:     "Olá do bZapper!",
	}))

	// Image (by URL)
	mustSend(client.SendImage(ctx, bzapper.SendMediaParams{
		SendBase: bzapper.SendBase{To: to},
		Media:    bzapper.MediaInput{URL: "https://example.com/cat.png", Caption: "gatinho"},
	}))

	// Video
	mustSend(client.SendVideo(ctx, bzapper.SendMediaParams{
		SendBase: bzapper.SendBase{To: to},
		Media:    bzapper.MediaInput{URL: "https://example.com/clip.mp4"},
	}))

	// Document
	mustSend(client.SendDocument(ctx, bzapper.SendMediaParams{
		SendBase: bzapper.SendBase{To: to},
		Media:    bzapper.MediaInput{URL: "https://example.com/invoice.pdf", Filename: "invoice.pdf"},
	}))

	// Audio (voice note via PTT)
	mustSend(client.SendAudio(ctx, bzapper.SendMediaParams{
		SendBase: bzapper.SendBase{To: to},
		Media:    bzapper.MediaInput{URL: "https://example.com/voice.ogg", PTT: true},
	}))

	// Sticker
	mustSend(client.SendSticker(ctx, bzapper.SendMediaParams{
		SendBase: bzapper.SendBase{To: to},
		Media:    bzapper.MediaInput{URL: "https://example.com/sticker.webp"},
	}))

	// Location
	mustSend(client.SendLocation(ctx, bzapper.SendLocationParams{
		SendBase:  bzapper.SendBase{To: to},
		Latitude:  -23.5505,
		Longitude: -46.6333,
		Name:      "São Paulo",
	}))

	// Contact
	mustSend(client.SendContact(ctx, bzapper.SendContactParams{
		SendBase:    bzapper.SendBase{To: to},
		ContactName: "Suporte bZapper",
	}))

	// Poll
	mustSend(client.SendPoll(ctx, bzapper.SendPollParams{
		SendBase:        bzapper.SendBase{To: to},
		Name:            "Qual seu canal favorito?",
		Options:         []string{"WhatsApp", "Email", "Telefone"},
		SelectableCount: 1,
	}))

	// Buttons (may fall back to a numbered text menu)
	mustSend(client.SendButtons(ctx, bzapper.SendButtonsParams{
		SendBase: bzapper.SendBase{To: to},
		Body:     "Escolha uma opção:",
		Buttons:  []bzapper.Button{{ID: "yes", Title: "Sim"}, {ID: "no", Title: "Não"}},
	}))

	// List (may fall back to a numbered text menu)
	mustSend(client.SendList(ctx, bzapper.SendListParams{
		SendBase:   bzapper.SendBase{To: to},
		Body:       "Cardápio:",
		ButtonText: "Ver opções",
		Sections: []bzapper.ListSection{{
			Title: "Bebidas",
			Rows:  []bzapper.ListRow{{ID: "coffee", Title: "Café", Description: "Quentinho"}},
		}},
	}))

	// Reaction (needs the wa_message_id you reply to)
	mustSend(client.SendReaction(ctx, bzapper.SendReactionParams{
		SendBase: bzapper.SendBase{To: to, QuotedMessageID: "ABCD1234"},
		Emoji:    "👍",
	}))

	// Instances
	if list, err := client.ListInstances(ctx); err == nil {
		fmt.Printf("instances: %d\n", list.Pagination.Total)

		// Advanced ops below need an instance id; use the first one if present.
		if len(list.Data) > 0 {
			instanceID := list.Data[0].ID

			// Presence works in groups too — send a "typing" indicator to a group.
			const groupJID = "120363021234567890@g.us"
			if err := client.PresenceChat(ctx, bzapper.PresenceChatParams{
				InstanceID: instanceID,
				To:         groupJID,
				State:      bzapper.PresenceTyping,
			}); err != nil {
				log.Printf("presence: %v", err)
			}

			// Conversations
			if convs, err := client.ListConversations(ctx, instanceID); err == nil {
				fmt.Printf("conversations: %d\n", convs.Pagination.Total)
			}

			// Groups
			if groups, err := client.ListGroups(ctx, instanceID); err == nil {
				fmt.Printf("groups: %d\n", groups.Pagination.Total)
			}

			// Contacts check
			if res, err := client.ContactsCheck(ctx, bzapper.ContactsCheckParams{
				InstanceID: instanceID,
				Phones:     []string{to},
			}); err == nil {
				fmt.Printf("contacts checked: %d\n", len(res.Data))
			}
		}
	}

	// Usage
	if usage, err := client.GetUsage(ctx, bzapper.GetUsageParams{}); err == nil {
		fmt.Printf("usage total: %d (delivery rate %.2f)\n", usage.Total, usage.DeliveryRate)
	}
}

func mustSend(msg *bzapper.Message, err error) {
	if err != nil {
		var apiErr *bzapper.Error
		if errors.As(err, &apiErr) {
			// Branch on the stable Code, never the message text.
			log.Fatalf("api error: code=%s status=%d msg=%q", apiErr.Code, apiErr.StatusCode, apiErr.Message)
		}
		log.Fatalf("transport error: %v", err)
	}
	fmt.Printf("queued: %s (%s)\n", msg.MessageID, msg.Status)
}
