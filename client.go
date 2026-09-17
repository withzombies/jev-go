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

// SystemOne answers each question independently against the same state. The
// effective payload after ExtraBody merging is validated, encoded once, and
// replayed unchanged on retries. Oversized requests fail before HTTP.
// Known answers are checked against the effective questions. Decode failures
// return *ResponseValidationError with the original buffered HTTP response.
func (c *Client) SystemOne(ctx context.Context, request Request, options ...RequestOption) (*Response, error) {
	body, kinds, err := c.prepareSystemOne(request)
	if err != nil {
		return nil, err
	}
	var response Response
	_, err = c.do(ctx, http.MethodPost, "/v1/systemone", body, options, func(raw *HTTPResponse) error {
		if err := json.Unmarshal(raw.Body, &response); err != nil {
			return responseValidationError(raw, err)
		}
		for name, kind := range kinds {
			answer, ok := response.Answers[name]
			if !ok {
				return responseValidationError(raw, at("answers."+name, fmt.Errorf("missing answer")))
			}
			if answer.answerType() != kind {
				return responseValidationError(raw, at("answers."+name+".type", fmt.Errorf("answer type does not match question")))
			}
		}
		response.HTTPResponse = raw
		response.RequestID = raw.RequestID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// SystemOneRaw evaluates a request and returns the fully buffered HTTP response
// without decoding answers. It applies the same request validation, byte cap,
// options and non-2xx error handling as SystemOne, and closes the network body.
func (c *Client) SystemOneRaw(ctx context.Context, request Request, options ...RequestOption) (*HTTPResponse, error) {
	body, _, err := c.prepareSystemOne(request)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodPost, "/v1/systemone", body, options, nil)
}

// ListModels lists models and aliases with buffered HTTP metadata. Invalid model
// fields return *ResponseValidationError. Options affect only this call.
func (c *Client) ListModels(ctx context.Context, options ...RequestOption) (*ModelsResponse, error) {
	var result *ModelsResponse
	_, err := c.do(ctx, http.MethodGet, "/v1/models", nil, options, func(raw *HTTPResponse) error {
		var err error
		result, err = decodeModels(raw)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func decodeModels(raw *HTTPResponse) (*ModelsResponse, error) {
	fields, err := requiredFields(raw.Body, "models")
	if err != nil {
		return nil, responseValidationError(raw, err)
	}
	var models []json.RawMessage
	if err := decodeAt(fields["models"], &models, "models"); err != nil {
		return nil, responseValidationError(raw, err)
	}
	result := &ModelsResponse{Models: make([]Model, 0, len(models)), HTTPResponse: raw, RequestID: raw.RequestID}
	for i, data := range models {
		path := "models." + strconv.Itoa(i)
		if _, err := requiredFields(data, "name", "description", "release_date"); err != nil {
			return nil, responseValidationError(raw, at(path, err))
		}
		var model Model
		if err := decodeAt(data, &model, path); err != nil {
			return nil, responseValidationError(raw, err)
		}
		result.Models = append(result.Models, model)
	}
	return result, nil
}

// ListModelsRaw returns the fully buffered models response without decoding it.
// Non-2xx responses still return *APIError. It leaves no response body to close.
func (c *Client) ListModelsRaw(ctx context.Context, options ...RequestOption) (*HTTPResponse, error) {
	return c.do(ctx, http.MethodGet, "/v1/models", nil, options, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, options []RequestOption, decode func(*HTTPResponse) error) (*HTTPResponse, error) {
	o, err := c.options(options)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		started := time.Now()
		response, err := c.attempt(ctx, method, path, body, o, attempt)
		if err == nil && decode != nil {
			err = decode(response)
		}
		c.log(ctx, slog.LevelInfo, "HTTP attempt", "method", method, "path", path, "attempt", attempt, "elapsed", time.Since(started), "error_type", fmt.Sprintf("%T", err))
		if err == nil {
			return response, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt >= o.retry.MaxRetries || !o.retry.retryable(err) {
			return nil, err
		}
		var api *APIError
		var headers http.Header
		if errors.As(err, &api) {
			headers = api.Headers
		} else if response != nil {
			headers = response.Headers
		}
		delay := retryDelay(o.retry, attempt, headers, time.Now(), rand.Float64()) //nolint:gosec // Retry jitter is not cryptography.
		c.log(ctx, slog.LevelInfo, "HTTP retry", "method", method, "path", path, "attempt", attempt+1, "delay", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) attempt(ctx context.Context, method, path string, body []byte, o requestOptions, attempt int) (*HTTPResponse, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jev: create request: %w", err)
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
	endpoint := method + " " + path
	c.logWire(ctx, "HTTP request", endpoint, request.Header, body)
	response, err := h.Do(request)
	if err != nil {
		return nil, &TransportError{Endpoint: endpoint, Err: err}
	}
	defer func() {
		// Reading determines success; closing only releases transport resources.
		_ = response.Body.Close()
	}()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, &TransportError{Endpoint: endpoint, Err: err}
	}
	raw := &HTTPResponse{StatusCode: response.StatusCode, Headers: response.Header.Clone(), Body: data, RequestID: response.Header.Get("x-typesafe-request-id"), Endpoint: endpoint}
	c.logWire(ctx, "HTTP response", endpoint, raw.Headers, data)
	c.log(ctx, slog.LevelInfo, "HTTP response", "endpoint", endpoint, "status", raw.StatusCode, "request_id", raw.RequestID)
	if raw.StatusCode < 200 || raw.StatusCode >= 300 {
		return nil, newAPIError(raw)
	}
	return raw, nil
}
