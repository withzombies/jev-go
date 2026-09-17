package jev

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ConfigFromEnv loads optional configuration using the caller's lookup function,
// typically os.Getenv. It never accesses the process environment itself. Blank
// values are ignored. It reads TYPESAFE_API_KEY, TYPESAFE_BASE_URL,
// TYPESAFE_DEFAULT_MODEL and TYPESAFE_LOG_LEVEL. The caller may override fields
// before [NewClient]; no logger is created and retries remain disabled.
// A nil lookup function or an invalid log level returns an error. Remaining
// configuration is validated by NewClient.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, fmt.Errorf("jev: environment lookup is nil")
	}
	cfg := Config{APIKey: strings.TrimSpace(getenv("TYPESAFE_API_KEY")), BaseURL: strings.TrimSpace(getenv("TYPESAFE_BASE_URL")), DefaultModel: strings.TrimSpace(getenv("TYPESAFE_DEFAULT_MODEL")), LogLevel: strings.ToLower(strings.TrimSpace(getenv("TYPESAFE_LOG_LEVEL")))}
	return cfg, validateLogLevel(cfg.LogLevel)
}

func validateLogLevel(level string) error {
	switch level {
	case "", "debug", "info", "warn", "error", "off":
		return nil
	default:
		return fmt.Errorf("jev: invalid log level %q", level)
	}
}

// RequestOption adjusts one call without modifying the client. Options returned
// by this package may be reused concurrently. Later options take precedence.
type RequestOption func(*requestOptions) error

type requestOptions struct {
	headers http.Header
	timeout time.Duration
	retry   RetryPolicy
}

// WithHeaders merges a snapshot of headers over the client defaults. Header names
// are case insensitive. Authentication, protocol and SDK headers stay protected.
func WithHeaders(headers http.Header) RequestOption {
	snapshot := mergeHeaders(nil, headers)
	return func(o *requestOptions) error { o.headers = mergeHeaders(o.headers, snapshot); return nil }
}

// WithTimeout sets a positive timeout for each complete HTTP attempt, including
// response body delivery. It may override the injected HTTP client's timeout;
// the caller's context still bounds the entire operation.
func WithTimeout(timeout time.Duration) RequestOption {
	return func(o *requestOptions) error {
		if timeout <= 0 {
			return fmt.Errorf("jev: timeout must be positive")
		}
		o.timeout = timeout
		return nil
	}
}

func mergeHeaders(base, override http.Header) http.Header {
	result := make(http.Header)
	for _, source := range []http.Header{base, override} {
		for name, values := range source {
			result[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
		}
	}
	return result
}

func (c *Client) options(options []RequestOption) (requestOptions, error) {
	o := requestOptions{headers: c.headers.Clone(), timeout: c.timeout, retry: c.retry}
	for _, option := range options {
		if option == nil {
			return o, fmt.Errorf("jev: nil request option")
		}
		if err := option(&o); err != nil {
			return o, err
		}
	}
	return o, nil
}
