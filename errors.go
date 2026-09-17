package jev

import (
	"fmt"
	"net/http"
)

// APIError is a non-2xx response. Body and Headers retain server diagnostics,
// including retry hints. Error omits the body because it may echo input content.
type APIError struct {
	// ErrorType is the best-effort detail.error_type value, or empty if unavailable.
	// The service uses "max_tokens_exceeded" for a context-limit rejection.
	ErrorType string
	// StatusCode is the HTTP response status.
	StatusCode int
	// Body holds the original response bytes and may contain submitted content.
	Body []byte
	// Headers is a cloned response header map, including any retry hints.
	Headers http.Header
	// RequestID is the x-typesafe-request-id response header, if present.
	RequestID string
}

// Error describes the status and request ID without exposing the response body.
func (e *APIError) Error() string {
	message := fmt.Sprintf("jev: HTTP %d", e.StatusCode)
	if status := http.StatusText(e.StatusCode); status != "" {
		message += " " + status
	}
	if e.ErrorType == "max_tokens_exceeded" {
		message += ": context limit exceeded (max_tokens_exceeded)"
	}
	if e.RequestID != "" {
		message += fmt.Sprintf(" (request %s)", e.RequestID)
	}
	return message
}

// RequestSizeError means the encoded request exceeded the caller's byte cap.
// It is a local size check, not a measurement of the model's token context.
type RequestSizeError struct {
	// Size is the actual encoded request length in bytes.
	Size int
	// Limit is the configured maximum request length in bytes.
	Limit int
}

// Error reports the measured byte count and configured limit.
func (e *RequestSizeError) Error() string {
	return fmt.Sprintf("jev: encoded request is %d bytes, exceeds limit of %d bytes", e.Size, e.Limit)
}
