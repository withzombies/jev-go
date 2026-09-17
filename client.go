package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.typesafe.ai"
	DefaultModel   = "jev-latest"
)

// Config explicitly configures a client; the library does not read environment
// variables. HTTPClient remains caller-owned and is never modified or closed.
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// Client can be reused concurrently. Callers must not mutate requests or the
// supplied HTTP client while calls are in progress. Requests are not retried.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewClient supplies defaults for an omitted BaseURL and HTTPClient.
func NewClient(cfg Config) (*Client, error) {
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
	return &Client{apiKey: key, baseURL: strings.TrimRight(base, "/"), httpClient: h}, nil
}

// SystemOne answers each question independently against the same state.
func (c *Client) SystemOne(ctx context.Context, request Request) (*Response, error) {
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
		request.Model = DefaultModel
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("jev: encode request: %w", err)
	}
	data, headers, err := c.do(ctx, http.MethodPost, "/v1/systemone", body)
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
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	data, _, err := c.do(ctx, http.MethodGet, "/v1/models", nil)
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

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("jev: create request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("jev: %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("jev: read response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, &APIError{StatusCode: response.StatusCode, Body: data, Headers: response.Header.Clone(), RequestID: response.Header.Get("x-typesafe-request-id")}
	}
	return data, response.Header, nil
}
