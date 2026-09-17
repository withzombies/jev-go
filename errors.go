package jev

import (
	"fmt"
	"net/http"
)

// APIError is a non-2xx response. Body and Headers retain server diagnostics,
// including retry hints. Error omits the body because it may echo input content.
type APIError struct {
	ErrorType  string
	StatusCode int
	Body       []byte
	Headers    http.Header
	RequestID  string
}

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
	Size  int
	Limit int
}

func (e *RequestSizeError) Error() string {
	return fmt.Sprintf("jev: encoded request is %d bytes, exceeds limit of %d bytes", e.Size, e.Limit)
}
