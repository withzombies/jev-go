package jev

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// APIError is a non-2xx response. Body and Headers retain server diagnostics,
// including retry hints. Error omits the body because it may echo input content.
type APIError struct {
	// Message is the best-effort server explanation. It may echo request data and
	// is deliberately excluded from Error(). Empty means no recognized message.
	Message string
	// Endpoint is the HTTP method and relative API path.
	Endpoint string
	// RetryAfter is the parsed server delay at response receipt. Nil means absent
	// or invalid; zero is a valid immediate retry hint. It does not enable retries.
	RetryAfter *time.Duration
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

// TransportError reports a connection or response-body delivery failure. Err is
// retained for errors.Is and errors.As, including context deadlines.
type TransportError struct {
	// Endpoint contains the HTTP method and relative API path.
	Endpoint string
	// Err is the underlying network or body-read error.
	Err error
}

// Error describes the failed operation and underlying cause.
func (e *TransportError) Error() string { return fmt.Sprintf("jev: %s: %v", e.Endpoint, e.Err) }

// Unwrap returns the underlying cause.
func (e *TransportError) Unwrap() error { return e.Err }

// Timeout reports whether the cause implements net.Error with Timeout true.
func (e *TransportError) Timeout() bool {
	var timeout net.Error
	return errors.As(e.Err, &timeout) && timeout.Timeout()
}

// ResponseValidationError reports an invalid successful response. HTTPResponse
// preserves the original bytes for diagnostics; Error never includes the body.
type ResponseValidationError struct {
	// FieldPath identifies the offending field, such as answers.name.confidence.
	// Empty denotes invalid top-level JSON. Dots separate object keys and indices.
	FieldPath string
	// HTTPResponse is the fully buffered response that failed validation.
	HTTPResponse *HTTPResponse
	// Err is the underlying JSON or structural validation error.
	Err error
}

// Error describes the invalid field without exposing server content.
func (e *ResponseValidationError) Error() string {
	message := fmt.Sprintf("jev: invalid response at %q", e.FieldPath)
	if e.HTTPResponse != nil && e.HTTPResponse.RequestID != "" {
		message += fmt.Sprintf(" (request %s)", e.HTTPResponse.RequestID)
	}
	return message
}

// Unwrap preserves the underlying decoding or validation cause.
func (e *ResponseValidationError) Unwrap() error { return e.Err }

func responseValidationError(response *HTTPResponse, err error) *ResponseValidationError {
	var field *fieldError
	path := ""
	if errors.As(err, &field) {
		path = field.path
	}
	return &ResponseValidationError{FieldPath: path, HTTPResponse: response, Err: err}
}

func newAPIError(response *HTTPResponse) *APIError {
	var body any
	if err := json.Unmarshal(response.Body, &body); err != nil {
		body = string(response.Body)
	}
	result := &APIError{StatusCode: response.StatusCode, Body: response.Body, Headers: response.Headers, RequestID: response.RequestID, Endpoint: response.Endpoint, Message: extractMessage(body)}
	if fields, ok := body.(map[string]any); ok {
		if detail, ok := fields["detail"].(map[string]any); ok {
			result.ErrorType, _ = detail["error_type"].(string)
		}
	}
	if delay, ok := parseRetryAfter(response.Headers, time.Now()); ok {
		result.RetryAfter = &delay
	}
	return result
}

func extractMessage(body any) string {
	if text, ok := body.(string); ok {
		return text
	}
	fields, ok := body.(map[string]any)
	if !ok {
		return ""
	}
	for _, name := range []string{"error", "message", "detail"} {
		switch value := fields[name].(type) {
		case string:
			return value
		case map[string]any:
			if message, ok := value["message"].(string); ok {
				return message
			}
		case []any:
			if name != "detail" {
				continue
			}
			var messages []string
			for _, item := range value {
				entry, ok := item.(map[string]any)
				if !ok {
					continue
				}
				message, ok := entry["msg"].(string)
				if !ok {
					continue
				}
				var path []string
				if locations, ok := entry["loc"].([]any); ok {
					for _, location := range locations {
						if location != "body" {
							path = append(path, fmt.Sprint(location))
						}
					}
				}
				if len(path) > 0 {
					message = strings.Join(path, ".") + ": " + message
				}
				messages = append(messages, message)
			}
			if len(messages) > 0 {
				return strings.Join(messages, "; ")
			}
		}
	}
	return ""
}
