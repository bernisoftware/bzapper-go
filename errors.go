package bzapper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Error is the typed error returned for non-2xx API responses. Always branch on
// Code (a stable, neutral identifier such as "instance_not_connected" or
// "rate_limited") — never parse Message, which is a human-readable, localized
// string.
type Error struct {
	// Code is the stable, neutral error code. Use this in your logic.
	Code string `json:"code"`
	// Message is the localized, human-readable message. Do not parse it.
	Message string `json:"message"`
	// Locale is the locale the Message was rendered in (e.g. "pt-BR").
	Locale string `json:"locale"`
	// StatusCode is the HTTP status code of the response.
	StatusCode int `json:"-"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("bzapper: %s (%s, http %d)", e.Message, e.Code, e.StatusCode)
	}
	if e.Message != "" {
		return fmt.Sprintf("bzapper: %s (http %d)", e.Message, e.StatusCode)
	}
	return fmt.Sprintf("bzapper: request failed with status %d", e.StatusCode)
}

// parseError builds an *Error from a non-2xx response body. When the body is
// not a recognizable error envelope, a best-effort *Error is still returned so
// callers can rely on the StatusCode.
func parseError(statusCode int, body []byte) *Error {
	e := &Error{StatusCode: statusCode}
	if len(body) > 0 {
		// Ignore decode failures: not every error body is a JSON envelope.
		_ = json.Unmarshal(body, e)
	}
	if e.Message == "" {
		e.Message = strings.TrimSpace(http.StatusText(statusCode))
	}
	return e
}
