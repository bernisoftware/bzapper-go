package bzapper

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrorType classifies an *Error by HTTP status (the same values in every
// official bZapper SDK). Use errors.Is with the sentinels (ErrNotFound,
// ErrRateLimit, …) or compare Error.Type.
type ErrorType string

const (
	// ErrorTypeAPI: any status without a dedicated type (e.g. 402, 418) and a 2xx
	// response whose body is not JSON (Code INVALID_RESPONSE).
	ErrorTypeAPI ErrorType = "api"
	// ErrorTypeAuthentication: 401 — missing, invalid or revoked key.
	ErrorTypeAuthentication ErrorType = "authentication"
	// ErrorTypePermissionDenied: 403 — e.g. a missing scope (see RequiredScope).
	ErrorTypePermissionDenied ErrorType = "permission_denied"
	// ErrorTypeNotFound: 404.
	ErrorTypeNotFound ErrorType = "not_found"
	// ErrorTypeConflict: 409.
	ErrorTypeConflict ErrorType = "conflict"
	// ErrorTypeValidation: 400 and 422.
	ErrorTypeValidation ErrorType = "validation"
	// ErrorTypeRateLimit: 429 — the suggested wait is in RetryAfter.
	ErrorTypeRateLimit ErrorType = "rate_limit"
	// ErrorTypeServer: 5xx.
	ErrorTypeServer ErrorType = "server"
	// ErrorTypeNetwork: connection failure or timeout (StatusCode 0, Code NETWORK_ERROR).
	ErrorTypeNetwork ErrorType = "network"
)

const (
	// CodeNetworkError is the Code of every network error (StatusCode 0).
	CodeNetworkError = "NETWORK_ERROR"
	// CodeInvalidResponse is the Code of a 2xx response whose body is not valid JSON.
	CodeInvalidResponse = "INVALID_RESPONSE"
)

// Error is the typed error returned for non-2xx API responses, for invalid 2xx
// responses (Code INVALID_RESPONSE) and for network failures (Code
// NETWORK_ERROR, StatusCode 0). Always branch on Code (a stable, neutral
// identifier such as "instance_not_connected" or "rate_limited") — never parse
// Message, which is a human-readable, localized string.
//
// For the category use errors.Is with the sentinels:
//
//	if errors.Is(err, bzapper.ErrRateLimit) { ... }
type Error struct {
	// Code is the stable, neutral error code. Use this in your logic. It comes
	// from body.code, else body.error, else "HTTP_<status>" (non-JSON body).
	Code string `json:"code"`
	// Message is the localized, human-readable message (body.message, else the
	// Code). Do not parse it.
	Message string `json:"message"`
	// Locale is the locale the Message was rendered in (e.g. "pt-BR").
	Locale string `json:"locale"`
	// StatusCode is the HTTP status code of the response (0 on network errors).
	StatusCode int `json:"-"`

	// Type classifies the error by status (ErrorTypeNotFound, …).
	Type ErrorType `json:"-"`
	// RequestID identifies the request — send it to support. It is the
	// X-Request-Id response header, else the X-Request-Id the SDK sent (the API
	// echoes the client's id, so both match the server logs).
	RequestID string `json:"-"`
	// RetryAfter is the wait requested by the API in the Retry-After header
	// (429 only; 0 otherwise).
	RetryAfter time.Duration `json:"-"`
	// RequiredScope is the scope the key lacks, from the X-Required-Scope header
	// (403 scope errors only).
	RequiredScope string `json:"-"`
	// Body is the decoded response body (structured detail of some errors): the
	// JSON value, or the raw text when the body is not JSON. nil when empty.
	Body any `json:"-"`
	// Err is the underlying cause (connection error, JSON decode error…), if any.
	Err error `json:"-"`
}

// Error implements the error interface. The string always carries the Code.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("bzapper: ")
	switch {
	case e.Message != "" && e.Code != "" && e.Message != e.Code:
		fmt.Fprintf(&b, "%s (%s, http %d", e.Message, e.Code, e.StatusCode)
	case e.Code != "":
		fmt.Fprintf(&b, "%s (http %d", e.Code, e.StatusCode)
	case e.Message != "":
		fmt.Fprintf(&b, "%s (http %d", e.Message, e.StatusCode)
	default:
		fmt.Fprintf(&b, "request failed (http %d", e.StatusCode)
	}
	if e.RequestID != "" {
		b.WriteString(", request_id ")
		b.WriteString(e.RequestID)
	}
	b.WriteString(")")
	if e.Err != nil && e.Type == ErrorTypeNetwork {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Unwrap returns the underlying cause (connection error, decode error…), if any.
func (e *Error) Unwrap() error { return e.Err }

// Is makes errors.Is(err, ErrNotFound) (and the other sentinels) match by Type.
func (e *Error) Is(target error) bool {
	s, ok := target.(*typeSentinel)
	return ok && s.typ == e.errorType()
}

// errorType returns Type, deriving it from StatusCode when unset (an *Error
// built by hand).
func (e *Error) errorType() ErrorType {
	if e.Type != "" {
		return e.Type
	}
	return typeForStatus(e.StatusCode)
}

func (e *Error) retryable() bool {
	if e.errorType() == ErrorTypeNetwork {
		return true
	}
	switch e.StatusCode {
	case 429, 502, 503, 504:
		return true
	}
	return false
}

type typeSentinel struct {
	typ  ErrorType
	text string
}

func (s *typeSentinel) Error() string { return s.text }

// Sentinels for errors.Is — each matches any *Error of the corresponding Type.
// They are the Go counterpart of the typed error classes of the other SDKs
// (AuthenticationError, NotFoundError, …).
//
//	if errors.Is(err, bzapper.ErrNotFound) { ... }
var (
	ErrAuthentication   error = &typeSentinel{ErrorTypeAuthentication, "bzapper: authentication error (401)"}
	ErrPermissionDenied error = &typeSentinel{ErrorTypePermissionDenied, "bzapper: permission denied (403)"}
	ErrNotFound         error = &typeSentinel{ErrorTypeNotFound, "bzapper: not found (404)"}
	ErrConflict         error = &typeSentinel{ErrorTypeConflict, "bzapper: conflict (409)"}
	ErrValidation       error = &typeSentinel{ErrorTypeValidation, "bzapper: validation error (400/422)"}
	ErrRateLimit        error = &typeSentinel{ErrorTypeRateLimit, "bzapper: rate limited (429)"}
	ErrServer           error = &typeSentinel{ErrorTypeServer, "bzapper: server error (5xx)"}
	ErrNetwork          error = &typeSentinel{ErrorTypeNetwork, "bzapper: network error"}
)

// ErrInvalidArgument is matched (errors.Is) by the argument errors the SDK
// returns BEFORE any request: an empty API key, or a path parameter that is
// empty, "." or "..". It is not an *Error.
var ErrInvalidArgument = errors.New("bzapper: invalid argument")

type argumentError struct{ msg string }

func (e *argumentError) Error() string        { return "bzapper: invalid argument: " + e.msg }
func (e *argumentError) Is(target error) bool { return target == ErrInvalidArgument }

func argErr(msg string) error { return &argumentError{msg: msg} }

func typeForStatus(status int) ErrorType {
	switch {
	case status == 0:
		return ErrorTypeNetwork
	case status == 401:
		return ErrorTypeAuthentication
	case status == 403:
		return ErrorTypePermissionDenied
	case status == 404:
		return ErrorTypeNotFound
	case status == 409:
		return ErrorTypeConflict
	case status == 400, status == 422:
		return ErrorTypeValidation
	case status == 429:
		return ErrorTypeRateLimit
	case status >= 500 && status <= 599:
		return ErrorTypeServer
	}
	return ErrorTypeAPI
}
