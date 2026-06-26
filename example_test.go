package bzapper_test

import (
	"context"
	"errors"
	"fmt"

	bzapper "github.com/bernisoftware/bzapper-go"
)

// Example shows the minimal "hello world": initialize the client and send a
// text message.
func Example() {
	client := bzapper.New("http://localhost:8080", "bz_live_...")

	msg, err := client.SendText(context.Background(), bzapper.SendTextParams{
		SendBase: bzapper.SendBase{To: "+5511999999999"},
		Body:     "Olá do bZapper!",
	})
	if err != nil {
		// Always branch on the stable error Code, never the message text.
		var apiErr *bzapper.Error
		if errors.As(err, &apiErr) {
			fmt.Println("api error:", apiErr.Code)
			return
		}
		fmt.Println("error:", err)
		return
	}
	fmt.Println("queued:", msg.MessageID)
}

// ExampleClient_SendImage sends an image by URL.
func ExampleClient_SendImage() {
	client := bzapper.New("http://localhost:8080", "bz_live_...")

	_, _ = client.SendImage(context.Background(), bzapper.SendMediaParams{
		SendBase: bzapper.SendBase{To: "+5511999999999"},
		Media:    bzapper.MediaInput{URL: "https://example.com/cat.png", Caption: "gatinho"},
	})
}
