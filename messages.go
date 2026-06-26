package bzapper

import (
	"context"
	"net/http"
)

// sendMessage posts a message-send request and decodes the queued envelope.
func (c *Client) sendMessage(ctx context.Context, path string, body any) (*Message, error) {
	var msg Message
	if err := c.do(ctx, http.MethodPost, path, nil, body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// SendText sends a text message. POST /messages/text.
func (c *Client) SendText(ctx context.Context, p SendTextParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/text", p)
}

// SendOTP sends a verification code as two messages — context text + the code on
// its own copyable bubble. Counts as one send. POST /messages/otp.
func (c *Client) SendOTP(ctx context.Context, p SendOTPParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/otp", p)
}

// SendImage sends an image (by URL or base64). POST /messages/image.
func (c *Client) SendImage(ctx context.Context, p SendMediaParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/image", p)
}

// SendVideo sends a video. POST /messages/video.
func (c *Client) SendVideo(ctx context.Context, p SendMediaParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/video", p)
}

// SendDocument sends a document. POST /messages/document.
func (c *Client) SendDocument(ctx context.Context, p SendMediaParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/document", p)
}

// SendAudio sends an audio message. Set Media.PTT to true for a voice note.
// POST /messages/audio.
func (c *Client) SendAudio(ctx context.Context, p SendMediaParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/audio", p)
}

// SendSticker sends a sticker. POST /messages/sticker.
func (c *Client) SendSticker(ctx context.Context, p SendMediaParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/sticker", p)
}

// SendLocation sends a location. POST /messages/location.
func (c *Client) SendLocation(ctx context.Context, p SendLocationParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/location", p)
}

// SendContact sends a contact (vCard). POST /messages/contact.
func (c *Client) SendContact(ctx context.Context, p SendContactParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/contact", p)
}

// SendPoll sends a poll. POST /messages/poll.
func (c *Client) SendPoll(ctx context.Context, p SendPollParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/poll", p)
}

// SendReaction reacts to a message. QuotedMessageID and Emoji are required.
// POST /messages/reaction.
func (c *Client) SendReaction(ctx context.Context, p SendReactionParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/reaction", p)
}

// SendButtons sends buttons. WhatsApp may fall back to a numbered text menu.
// POST /messages/buttons.
func (c *Client) SendButtons(ctx context.Context, p SendButtonsParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/buttons", p)
}

// SendList sends a list. WhatsApp may fall back to a numbered text menu.
// POST /messages/list.
func (c *Client) SendList(ctx context.Context, p SendListParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/list", p)
}
