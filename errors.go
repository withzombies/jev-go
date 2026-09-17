package jev

import (
	"fmt"
	"net/http"
)

// APIError is a non-2xx response. Body and Headers retain server diagnostics,
// including retry hints. Error omits the body because it may echo input content.
type APIError struct {
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
	if e.RequestID != "" {
		message += fmt.Sprintf(" (request %s)", e.RequestID)
	}
	return message
}
