package bzapper

import (
	"context"
	"net/http"
	"net/url"
)

// AIConnection is an AI app (MCP client) a person authorized to operate this
// account through the AI connector.
type AIConnection struct {
	ID string `json:"id"`
	// ClientName is the name the AI app registered with, e.g. "Claude".
	ClientName string `json:"client_name,omitempty"`
	// UserID is the person who authorized it.
	UserID    string `json:"user_id,omitempty"`
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
	// UserRole is the person's role now ("admin" or "agent"); the connection never
	// exceeds it.
	UserRole string `json:"user_role,omitempty"`
	// ProjectIDs are the projects the connection reaches.
	ProjectIDs []string `json:"project_ids,omitempty"`
	// Scopes granted: bzapper:read (always), bzapper:contacts, bzapper:send,
	// bzapper:campaigns.
	Scopes    []string `json:"scopes,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	// LastUsedAt is empty until the first call.
	LastUsedAt string `json:"last_used_at,omitempty"`
}

// aiConnectionList mirrors the {"data": [...]} list envelope.
type aiConnectionList struct {
	Data []AIConnection `json:"data"`
}

// ListAIConnections lists the active connections of the AI connector, newest
// first. An admin sees every connection of the account; an agent only their own;
// an API key without a person behind it sees them only if it is an admin key.
// GET /ai-connections.
func (c *Client) ListAIConnections(ctx context.Context) ([]AIConnection, error) {
	var out aiConnectionList
	if err := c.do(ctx, http.MethodGet, "/ai-connections", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// RevokeAIConnection revokes an AI app's access; its tokens stop working on the
// next call. An admin can revoke any connection of the account; an agent only
// their own. DELETE /ai-connections/{id}.
func (c *Client) RevokeAIConnection(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/ai-connections/"+url.PathEscape(id), nil, nil, nil)
}
