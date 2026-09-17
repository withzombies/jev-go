package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the public TypeSafe API endpoint.
	DefaultBaseURL = "https://api.typesafe.ai"
	// DefaultModel follows TypeSafe's current stable Jev alias.
	DefaultModel = "jev-latest"
)

// Config explicitly configures a client; the library does not read environment
// variables. HTTPClient remains caller-owned and is never modified or closed.
type Config struct {
	// Retry is an opt-in policy. Its zero value disables retries.
	Retry RetryPolicy
	// LogBodies includes request and response contents at debug level. False
	// omits bodies even with a debug logger. Contents may contain sensitive data.
	LogBodies bool
	// DefaultModel is used when Request.Model is empty; empty selects DefaultModel.
	DefaultModel string
	// Headers supplies additional request headers; NewClient snapshots the map.
	Headers http.Header
	// Timeout overrides the HTTP client timeout per attempt. Zero inherits the
	// supplied client timeout, or ten seconds without a supplied client.
	Timeout time.Duration
	// Logger receives optional diagnostics. Nil disables all logging.
	Logger *slog.Logger
	// LogLevel filters diagnostics: debug, info, warn (default), error, or off.
	LogLevel string

	// APIKey is the required bearer credential. Surrounding whitespace is trimmed.
	APIKey string
	// BaseURL defaults to DefaultBaseURL. It must be an absolute HTTP(S) URL
	// without credentials, query parameters, or a fragment.
	BaseURL string
	// HTTPClient supplies the transport and timeout. Nil uses a new client with
	// a ten-second timeout. A supplied client is never modified or closed.
	HTTPClient *http.Client
	// MaxRequestBytes caps the complete encoded request, including JSON escaping,
	// questions, and model. Zero disables the cap; negative values are invalid.
	// This byte count is not a token count or a guarantee of server acceptance.
	MaxRequestBytes int
}

// Client can be reused concurrently. Callers must not mutate requests or the
// supplied HTTP client while calls are in progress. Retries require an explicit policy.
// The zero value is not usable; construct a Client with NewClient.
type Client struct {
	apiKey          string
	baseURL         string
	httpClient      *http.Client
	maxRequestBytes int
	defaultModel    string
	headers         http.Header
	timeout         time.Duration
	logger          *slog.Logger
	logLevel        string
	logBodies       bool
	retry           RetryPolicy
}

// NewClient validates configuration and supplies defaults for an omitted BaseURL
// and HTTPClient. It performs no network requests. An empty API key, invalid
// base URL, or negative byte limit returns an error.
func NewClient(cfg Config) (*Client, error) {
	if err := cfg.Retry.validate(); err != nil {
		return nil, err
	}
	if cfg.Timeout < 0 {
		return nil, fmt.Errorf("jev: timeout must not be negative")
	}
	if err := validateLogLevel(cfg.LogLevel); err != nil {
		return nil, err
	}
	if cfg.MaxRequestBytes < 0 {
		return nil, fmt.Errorf("jev: MaxRequestBytes must not be negative")
	}
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return nil, fmt.Errorf("jev: a nonempty, single-line API key is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, fmt.Errorf("jev: base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: 10 * time.Second}
	}
	model := cfg.DefaultModel
	if model == "" {
		model = DefaultModel
	}
	return &Client{apiKey: key, baseURL: strings.TrimRight(base, "/"), httpClient: h, maxRequestBytes: cfg.MaxRequestBytes, defaultModel: model, headers: mergeHeaders(nil, cfg.Headers), timeout: cfg.Timeout, logger: cfg.Logger, logLevel: cfg.LogLevel, logBodies: cfg.LogBodies, retry: cfg.Retry.clone()}, nil
}

// SystemOne answers each question independently against the same state.
// It does not modify or truncate the supplied request; retries are opt-in.
// At least one non-nil question is required. A locally oversized request returns
// a *RequestSizeError before HTTP; a non-2xx response returns an *APIError.
// Transport errors preserve their causes, including context cancellation.
// Malformed responses, missing answers, and mismatched answer types are errors.
// Other model-specific input constraints are enforced by the service.
func (c *Client) SystemOne(ctx context.Context, request Request, options ...RequestOption) (*Response, error) {
	if len(request.Questions) == 0 {
		return nil, fmt.Errorf("jev: at least one question is required")
	}
	for name, q := range request.Questions {
		encoded, err := json.Marshal(q)
		if err != nil {
			return nil, fmt.Errorf("jev: encode question %q: %w", name, err)
		}
		if bytes.Equal(encoded, []byte("null")) {
			return nil, fmt.Errorf("jev: question %q is nil", name)
		}
	}
	if request.Model == "" {
		request.Model = c.defaultModel
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("jev: encode request: %w", err)
	}
	if c.maxRequestBytes > 0 && len(body) > c.maxRequestBytes {
		return nil, &RequestSizeError{Size: len(body), Limit: c.maxRequestBytes}
	}
	data, headers, err := c.do(ctx, http.MethodPost, "/v1/systemone", body, options)
	if err != nil {
		return nil, err
	}
	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("jev: decode evaluation: %w", err)
	}
	for name, q := range request.Questions {
		a, ok := response.Answers[name]
		if !ok {
			return nil, fmt.Errorf("jev: missing answer %q", name)
		}
		if a.answerType() != q.questionType() {
			return nil, fmt.Errorf("jev: answer %q has type %s, want %s", name, a.answerType(), q.questionType())
		}
	}
	response.RequestID = headers.Get("x-typesafe-request-id")
	return &response, nil
}

// ListModels lists models and aliases available to the authenticated account.
// It respects ctx and returns *APIError for non-2xx responses after any retries.
// Transport errors preserve their causes; malformed responses return errors.
func (c *Client) ListModels(ctx context.Context, options ...RequestOption) ([]Model, error) {
	data, _, err := c.do(ctx, http.MethodGet, "/v1/models", nil, options)
	if err != nil {
		return nil, err
	}
	if _, err := requiredFields(data, "models"); err != nil {
		return nil, fmt.Errorf("jev: decode models: %w", err)
	}
	var response struct {
		Models []Model `json:"models"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("jev: decode models: %w", err)
	}
	return response.Models, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, options []RequestOption) ([]byte, http.Header, error) {
	o, err := c.options(options)
	if err != nil {
		return nil, nil, err
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		started := time.Now()
		data, headers, err := c.attempt(ctx, method, path, body, o, attempt)
		c.log(ctx, slog.LevelInfo, "HTTP attempt", "method", method, "path", path, "attempt", attempt, "elapsed", time.Since(started), "error_type", fmt.Sprintf("%T", err))
		if err == nil {
			return data, headers, nil
		}
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if attempt >= o.retry.MaxRetries || !o.retry.retryable(err) {
			return nil, nil, err
		}
		var api *APIError
		if errors.As(err, &api) {
			headers = api.Headers
		}
		delay := retryDelay(o.retry, attempt, headers, time.Now(), rand.Float64()) //nolint:gosec // Retry jitter is not cryptography.
		c.log(ctx, slog.LevelInfo, "HTTP retry", "method", method, "path", path, "attempt", attempt+1, "delay", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) attempt(ctx context.Context, method, path string, body []byte, o requestOptions, attempt int) ([]byte, http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("jev: create request: %w", err)
	}
	request.Header = o.headers.Clone()
	request.Header.Set("User-Agent", "jev-go")
	request.Header.Set("X-TypeSafe-SDK", "jev-go")
	request.Header.Set("X-TypeSafe-Runtime", runtime.Version()+" ("+runtime.GOOS+"; "+runtime.GOARCH+")")
	request.Header.Del("X-TypeSafe-Retry-Count")
	if attempt > 0 {
		request.Header.Set("X-TypeSafe-Retry-Count", strconv.Itoa(attempt))
	}
	request.Header.Del("Content-Type")
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	h := c.httpClient
	if o.timeout > 0 {
		copyClient := *h
		copyClient.Timeout = o.timeout
		h = &copyClient
	}
	c.logWire(ctx, "HTTP request", method+" "+path, request.Header, body)
	response, err := h.Do(request)
	if err != nil {
		return nil, nil, &TransportError{Endpoint: method + " " + path, Err: err}
	}
	defer func() {
		// Reading determines success; closing only releases transport resources.
		_ = response.Body.Close()
	}()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, &TransportError{Endpoint: method + " " + path, Err: err}
	}
	c.logWire(ctx, "HTTP response", method+" "+path, response.Header, data)
	c.log(ctx, slog.LevelInfo, "HTTP response", "endpoint", method+" "+path, "status", response.StatusCode, "request_id", response.Header.Get("x-typesafe-request-id"))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var detail struct {
			Detail struct {
				ErrorType string `json:"error_type"`
			} `json:"detail"`
		}
		// Error decoding is best effort; always preserve the HTTP diagnostics.
		_ = json.Unmarshal(data, &detail)
		return nil, nil, &APIError{ErrorType: detail.Detail.ErrorType, StatusCode: response.StatusCode, Body: data, Headers: response.Header.Clone(), RequestID: response.Header.Get("x-typesafe-request-id")}
	}
	return data, response.Header, nil
}
