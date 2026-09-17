package jev_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	jev "github.com/withzombies/jev-go"
)

func ExampleClient_SystemOne() {
	// A local server keeps this executable example independent of credentials.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, `{"model":"jev-test","usage":{"input_tokens":10,"output_tokens":2},"answers":{"billing":{"type":"noul","noul":0.98}}}`); err != nil {
			panic(err)
		}
	}))
	defer server.Close()
	client, err := jev.NewClient(jev.Config{APIKey: "example-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		panic(err)
	}
	response, err := client.SystemOne(context.Background(), jev.Request{
		State: "I was charged twice.",
		Questions: map[string]jev.Question{
			"billing": jev.Noul{Instructions: "Is this message about billing?"},
		},
	})
	if err != nil {
		panic(err)
	}
	answer := response.Answers["billing"].(jev.NoulAnswer)
	fmt.Printf("Probability of billing: %.2f\n", answer.Noul)
	// Output: Probability of billing: 0.98
}

func ExampleNewClient() {
	httpClient := &http.Client{Timeout: 2 * time.Second}
	client, err := jev.NewClient(jev.Config{
		APIKey:          "example-key",
		HTTPClient:      httpClient,
		MaxRequestBytes: 64 * 1024,
	})
	if err != nil {
		panic(err)
	}
	// Construction makes no network request and leaves the supplied client intact.
	fmt.Println("Ready:", client != nil)
	fmt.Println("Timeout:", httpClient.Timeout)
	// Output:
	// Ready: true
	// Timeout: 2s
}

func ExampleClient_ListModels() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, `{"models":[{"name":"jev-example","description":"Example model","release_date":"2026-09-17"}]}`); err != nil {
			panic(err)
		}
	}))
	defer server.Close()
	client, err := jev.NewClient(jev.Config{APIKey: "example-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		panic(err)
	}
	models, err := client.ListModels(context.Background())
	if err != nil {
		panic(err)
	}
	for _, model := range models.Models {
		fmt.Println(model.Name, model.Description)
	}
	// Output: jev-example Example model
}

func ExampleRequestSizeError() {
	client, err := jev.NewClient(jev.Config{APIKey: "example-key", MaxRequestBytes: 1})
	if err != nil {
		panic(err)
	}
	// This request is rejected locally; no HTTP request is made.
	_, err = client.SystemOne(context.Background(), jev.Request{
		State:     "sample",
		Questions: map[string]jev.Question{"ok": jev.Noul{Instructions: "Is the state valid?"}},
	})
	var sizeErr *jev.RequestSizeError
	if errors.As(err, &sizeErr) {
		fmt.Println("Over budget:", sizeErr.Size > sizeErr.Limit)
	} else {
		panic(fmt.Sprintf("expected size rejection, got %v", err))
	}
	// Output: Over budget: true
}

func ExampleAPIError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-typesafe-request-id", "req-example")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := fmt.Fprint(w, `{"detail":{"error_type":"max_tokens_exceeded"}}`); err != nil {
			panic(err)
		}
	}))
	defer server.Close()
	client, err := jev.NewClient(jev.Config{APIKey: "example-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		panic(err)
	}
	_, err = client.SystemOne(context.Background(), jev.Request{
		State:     "sample",
		Questions: map[string]jev.Question{"ok": jev.Noul{Instructions: "Is the state valid?"}},
	})
	var apiErr *jev.APIError
	if errors.As(err, &apiErr) {
		// Inspect metadata without logging a body that could echo submitted content.
		fmt.Println(apiErr.StatusCode, apiErr.ErrorType, apiErr.RequestID)
	} else {
		panic(fmt.Sprintf("expected API rejection, got %v", err))
	}
	// Output: 400 max_tokens_exceeded req-example
}

func ExampleConfigFromEnv() {
	values := map[string]string{"TYPESAFE_API_KEY": "example-key", "TYPESAFE_DEFAULT_MODEL": "jev-latest"}
	cfg, err := jev.ConfigFromEnv(func(name string) string { return values[name] })
	if err != nil {
		panic(err)
	}
	// Applications can pass os.Getenv and then override individual fields.
	cfg.DefaultModel = "pinned-model"
	fmt.Println(cfg.DefaultModel, cfg.Logger == nil, cfg.Retry.MaxRetries)
	// Output: pinned-model true 0
}

func ExampleDefaultRetryPolicy() {
	policy := jev.DefaultRetryPolicy()
	policy.MaxRetries = 3
	policy.HTTPStatuses = []int{429, 503}
	fmt.Println(policy.MaxRetries, policy.BackoffInitial, policy.BackoffMax)
	fmt.Println(policy.HTTPStatuses)
	// Pass policy in Config.Retry or WithRetry(policy). A context deadline bounds
	// the whole call; WithRetry(jev.RetryPolicy{}) disables retries for one call.

	// Output:
	// 3 500ms 5s
	// [429 503]
}

func ExampleRawQuestion() {
	question := jev.RawQuestion{"type": "noul", "instructions": nil, "future_option": true}
	encoded, err := json.Marshal(question)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(encoded))
	// Output: {"future_option":true,"instructions":null,"type":"noul"}
}

func ExampleClient_SystemOneRaw() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-typesafe-request-id", "example-request")
		if _, err := fmt.Fprint(w, `{"future_response":true}`); err != nil {
			panic(err)
		}
	}))
	defer server.Close()
	client, err := jev.NewClient(jev.Config{APIKey: "example-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		panic(err)
	}
	response, err := client.SystemOneRaw(context.Background(), jev.Request{State: "example", Questions: map[string]jev.Question{"ok": jev.Noul{Instructions: "Acceptable?"}}}, jev.WithTimeout(time.Second))
	if err != nil {
		panic(err)
	}
	// The body is buffered and the network resource has already been closed.
	fmt.Println(response.StatusCode, response.RequestID, string(response.Body))
	// Output: 200 example-request {"future_response":true}
}

func ExampleResponse_Nouls() {
	var response jev.Response
	if err := json.Unmarshal([]byte(`{"model":"jev","usage":{},"answers":{"ok":{"type":"noul","noul":0.9}}}`), &response); err != nil {
		panic(err)
	}
	fmt.Println(response.Nouls()["ok"].Noul)
	fmt.Println(response.Usage.InputTokens == nil)
	// Output:
	// 0.9
	// true
}
