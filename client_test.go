package jev_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jev "github.com/withzombies/jev-go"
)

const oneResponse = `{"model":"jev-test","usage":{"input_tokens":5,"output_tokens":2},"answers":{"ok":{"type":"noul","noul":0.9}}}`

func clientFor(t *testing.T, handler http.HandlerFunc) *jev.Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	c, err := jev.NewClient(jev.Config{APIKey: "test-key", BaseURL: s.URL + "/", HTTPClient: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func oneRequest() jev.Request {
	return jev.Request{State: "patch", Questions: map[string]jev.Question{"ok": jev.Noul{Instructions: "Correct?"}}}
}

func TestSystemOneWireRequest(t *testing.T) {
	for _, model := range []string{"", "pinned-model"} {
		t.Run(model, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/v1/systemone" {
					t.Errorf("endpoint: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
					t.Error("incorrect request headers")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				requests <- body
				w.Header().Set("x-typesafe-request-id", "req-test")
				writeResponse(t, w, oneResponse)
			})
			req := oneRequest()
			req.State = map[string]any{"diff": "patch"}
			req.Model = model
			response, err := c.SystemOne(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if response.RequestID != "req-test" || response.Model != "jev-test" {
				t.Fatalf("metadata: %+v", response)
			}
			body := <-requests
			wantModel := model
			if wantModel == "" {
				wantModel = "jev-latest"
			}
			if body["model"] != wantModel || body["state"].(map[string]any)["diff"] != "patch" {
				t.Fatalf("body: %#v", body)
			}
			q := body["questions"].(map[string]any)["ok"].(map[string]any)
			if q["type"] != "noul" || q["instructions"] != "Correct?" {
				t.Fatalf("question: %#v", q)
			}
			if req.Model != model {
				t.Fatal("mutated request")
			}
		})
	}
}

func TestListModels(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("incorrect model request")
		}
		writeResponse(t, w, `{"models":[{"name":"jev-latest","description":"stable","release_date":"2026-09-10T00:00:00Z"}]}`)
	})
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Name != "jev-latest" || models[0].Description != "stable" || models[0].ReleaseDate != "2026-09-10T00:00:00Z" {
		t.Fatalf("models: %+v", models)
	}
}

func TestClientConfigValidation(t *testing.T) {
	for _, cfg := range []jev.Config{
		{}, {APIKey: "  "}, {APIKey: "bad\nkey"},
		{APIKey: "key", BaseURL: "relative"}, {APIKey: "key", BaseURL: "ftp://example.com"},
		{APIKey: "key", BaseURL: "https://user:pass@example.com"}, //nolint:gosec // Dummy credentials exercise URL rejection.
		{APIKey: "key", BaseURL: "https://example.com?x=1"},
		{APIKey: "key", BaseURL: "https://example.com#fragment"},
	} {
		if _, err := jev.NewClient(cfg); err == nil {
			t.Error("accepted invalid configuration")
		}
	}
}

func TestSystemOneRejectsInvalidRequestsBeforeHTTP(t *testing.T) {
	c := clientFor(t, func(_ http.ResponseWriter, _ *http.Request) { t.Error("invalid request reached server") })
	var nilQuestion *jev.Noul
	requests := []jev.Request{
		{State: "x"}, {State: "x", Questions: map[string]jev.Question{"bad": nil}},
		{State: "x", Questions: map[string]jev.Question{"bad": nilQuestion}},
		{State: make(chan int), Questions: oneRequest().Questions},
		{State: "x", Questions: map[string]jev.Question{"bad": jev.Noul{Instructions: make(chan int)}}},
	}
	for _, req := range requests {
		if _, err := c.SystemOne(context.Background(), req); err == nil {
			t.Error("expected error")
		}
	}
}

func TestSystemOneRejectsMissingOrWrongAnswers(t *testing.T) {
	for _, body := range []string{
		`{"model":"jev","usage":{"input_tokens":1,"output_tokens":1},"answers":{}}`,
		`{"model":"jev","usage":{"input_tokens":1,"output_tokens":1},"answers":{"ok":{"type":"choice","choice":"x","confidence":1,"probabilities":{"x":1}}}}`,
		`not JSON`, oneResponse + `{}`, `null`,
	} {
		c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, body) })
		if _, err := c.SystemOne(context.Background(), oneRequest()); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestListModelsRejectsMalformedResponse(t *testing.T) {
	for _, body := range []string{`{}`, `{"models":null}`, `{"models":"bad"}`, `garbage`} {
		c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, body) })
		if _, err := c.ListModels(context.Background()); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestAPIErrorsPreserveMetadataWithoutRetrying(t *testing.T) {
	for _, status := range []int{401, 422, 429, 529} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			body := `{"detail":"invalid"}`
			if status == 529 {
				body = "overloaded"
			}
			c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "2")
				w.Header().Set("x-typesafe-request-id", "req-error")
				w.WriteHeader(status)
				writeResponse(t, w, body)
			})
			_, err := c.SystemOne(context.Background(), oneRequest())
			var apiErr *jev.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected APIError, got %v", err)
			}
			if apiErr.StatusCode != status || string(apiErr.Body) != body || apiErr.RequestID != "req-error" || apiErr.Headers.Get("Retry-After") != "2" {
				t.Fatalf("metadata: %#v", apiErr)
			}
			if calls.Load() != 1 {
				t.Fatalf("made %d attempts", calls.Load())
			}
			if strings.Contains(err.Error(), "test-key") || !strings.Contains(err.Error(), fmt.Sprint(status)) {
				t.Fatalf("error: %s", err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestInjectedHTTPClientAndBodyClosure(t *testing.T) {
	readErr := errors.New("read interrupted")
	for _, tc := range []struct {
		name    string
		status  int
		reader  io.Reader
		wantErr bool
	}{
		{"success", 200, strings.NewReader(oneResponse), false},
		{"http failure", 500, strings.NewReader("failed"), true},
		{"decode failure", 200, strings.NewReader("invalid"), true},
		{"read failure", 200, errorReader{readErr}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: tc.reader}
			httpClient := &http.Client{Timeout: 7 * time.Second, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://api.typesafe.ai/v1/systemone" {
					t.Fatalf("URL: %s", r.URL)
				}
				return &http.Response{StatusCode: tc.status, Body: body, Header: make(http.Header)}, nil
			})}
			client, err := jev.NewClient(jev.Config{APIKey: "test-key", HTTPClient: httpClient})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.SystemOne(context.Background(), oneRequest())
			if (err != nil) != tc.wantErr {
				t.Fatalf("error: %v", err)
			}
			if tc.name == "read failure" && !errors.Is(err, readErr) {
				t.Fatalf("lost cause: %v", err)
			}
			if !body.closed {
				t.Error("body not closed")
			}
			if httpClient.Timeout != 7*time.Second {
				t.Error("changed supplied client")
			}
		})
	}
}

func TestTransportErrorIsWrapped(t *testing.T) {
	want := errors.New("offline")
	c, err := jev.NewClient(jev.Config{APIKey: "key", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, want })}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListModels(context.Background()); !errors.Is(err, want) {
		t.Fatalf("lost cause: %v", err)
	}
}

func TestCancellationAndHTTPTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { close(started); <-release }))
			defer s.Close()
			defer close(release)
			h := s.Client()
			if timeout {
				h.Timeout = 30 * time.Millisecond
			}
			c, err := jev.NewClient(jev.Config{APIKey: "key", BaseURL: s.URL, HTTPClient: h})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := c.ListModels(ctx); done <- err }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not start")
			}
			if !timeout {
				cancel()
			}
			select {
			case err := <-done:
				want := context.Canceled
				if timeout {
					want = context.DeadlineExceeded
				}
				if !errors.Is(err, want) {
					t.Fatalf("want %v, got %v", want, err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("request did not stop")
			}
		})
	}
}

func TestClientConcurrentReuse(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, oneResponse) })
	req := oneRequest()
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			if _, err := c.SystemOne(context.Background(), req); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if req.Model != "" {
		t.Error("changed shared request")
	}
}

func TestRequestByteLimit(t *testing.T) {
	request := oneRequest()
	request.State = "<escaped>\n☃"
	wire := request
	wire.Model = jev.DefaultModel
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{0, len(encoded), len(encoded) - 1} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				if string(body) != string(encoded) {
					t.Errorf("unexpected wire request: %s", body)
				}
				writeResponse(t, w, oneResponse)
			}))
			defer server.Close()
			client, err := jev.NewClient(jev.Config{APIKey: "test", BaseURL: server.URL, MaxRequestBytes: limit})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.SystemOne(context.Background(), request)
			if limit > 0 && limit < len(encoded) {
				var sizeErr *jev.RequestSizeError
				if !errors.As(err, &sizeErr) || sizeErr.Size != len(encoded) || sizeErr.Limit != limit {
					t.Fatalf("size error: %v", err)
				}
				if calls.Load() != 0 {
					t.Fatal("oversized request reached transport")
				}
			} else if err != nil || calls.Load() != 1 {
				t.Fatalf("request: calls=%d error=%v", calls.Load(), err)
			}
			if request.Model != "" {
				t.Fatal("request mutated")
			}
		})
	}
	if _, err := jev.NewClient(jev.Config{APIKey: "test", MaxRequestBytes: -1}); err == nil {
		t.Fatal("accepted negative limit")
	}
}

func TestAPIErrorType(t *testing.T) {
	for _, body := range []string{`{"detail":{"error_type":"max_tokens_exceeded"}}`, `{"detail":{"error_type":"other"}}`, `{"detail":{"error_type":7}}`, `not json`, `{}`} {
		t.Run(body, func(t *testing.T) {
			client := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("x-typesafe-request-id", "req-limit")
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(400)
				writeResponse(t, w, body)
			})
			_, err := client.SystemOne(context.Background(), oneRequest())
			var apiErr *jev.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error: %v", err)
			}
			want := ""
			if strings.Contains(body, `"max_tokens_exceeded"`) {
				want = "max_tokens_exceeded"
			}
			if strings.Contains(body, `"other"`) {
				want = "other"
			}
			if apiErr.ErrorType != want || apiErr.StatusCode != 400 || string(apiErr.Body) != body || apiErr.RequestID != "req-limit" || apiErr.Headers.Get("Retry-After") != "2" {
				t.Fatalf("diagnostics: %+v", apiErr)
			}
			if want == "max_tokens_exceeded" && !strings.Contains(err.Error(), "max_tokens_exceeded") {
				t.Fatalf("unhelpful error: %v", err)
			}
			if strings.Contains(err.Error(), body) {
				t.Fatal("error echoed response body")
			}
		})
	}
}

func writeResponse(t *testing.T, w io.Writer, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write response: %v", err)
	}
}
