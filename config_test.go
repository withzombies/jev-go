package jev_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jev "github.com/withzombies/jev-go"
)

func TestConfigFromEnv(t *testing.T) {
	values := map[string]string{"TYPESAFE_API_KEY": " key ", "TYPESAFE_BASE_URL": " https://example.test ", "TYPESAFE_DEFAULT_MODEL": " pinned ", "TYPESAFE_LOG_LEVEL": " debug "}
	cfg, err := jev.ConfigFromEnv(func(name string) string { return values[name] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "key" || cfg.BaseURL != "https://example.test" || cfg.DefaultModel != "pinned" || cfg.LogLevel != "debug" || cfg.Logger != nil {
		t.Fatalf("environment resolution failed: model=%q level=%q", cfg.DefaultModel, cfg.LogLevel)
	}
	values["TYPESAFE_LOG_LEVEL"] = "invalid"
	if _, err := jev.ConfigFromEnv(func(name string) string { return values[name] }); err == nil {
		t.Fatal("accepted invalid log level")
	}
	if _, err := jev.ConfigFromEnv(nil); err == nil {
		t.Fatal("accepted nil lookup")
	}
	cfg, err = jev.ConfigFromEnv(func(string) string { return " \t" })
	if err != nil || cfg.APIKey != "" || cfg.DefaultModel != "" {
		t.Fatal("blank environment was not ignored")
	}
}

func TestClientDefaultsAndCallHeaders(t *testing.T) {
	headers := http.Header{"x-custom": {"default"}, "authorization": {"wrong"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("Accept") != "application/json" || r.Header.Get("X-TypeSafe-SDK") == "" || r.Header.Get("X-TypeSafe-Runtime") == "" || r.Header.Get("X-TypeSafe-Retry-Count") != "" {
			t.Error("protected headers incorrect")
		}
		if r.Header.Get("X-Custom") != r.Header.Get("X-Expected") {
			t.Error("custom header precedence failed")
		}
		if r.Method == http.MethodGet {
			if r.Header.Get("Content-Type") != "" {
				t.Error("GET sent content type")
			}
			writeResponse(t, w, `{"models":[]}`)
			return
		}
		var req jev.Request
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal(body["model"], &req.Model); err != nil {
			t.Error(err)
		}
		if req.Model != r.Header.Get("X-Model") {
			t.Errorf("model=%s", req.Model)
		}
		writeResponse(t, w, oneResponse)
	}))
	defer server.Close()
	c, err := jev.NewClient(jev.Config{APIKey: "key", BaseURL: server.URL, DefaultModel: "pinned", Headers: headers})
	if err != nil {
		t.Fatal(err)
	}
	headers["x-custom"][0] = "mutated" //nolint:staticcheck // Exercise snapshotting of deliberately noncanonical caller headers.
	perCall := http.Header{"x-custom": {"call"}, "X-Expected": {"call"}, "X-Model": {"pinned"}, "AUTHORIZATION": {"bad"}, "CONTENT-TYPE": {"bad"}, "X-TypeSafe-Retry-Count": {"999"}}
	option := jev.WithHeaders(perCall)
	perCall["x-custom"][0] = "mutated" //nolint:staticcheck // Exercise snapshotting of deliberately noncanonical caller headers.
	if _, err := c.SystemOne(t.Context(), oneRequest(), option); err != nil {
		t.Fatal(err)
	}
	req := oneRequest()
	req.Model = "override"
	if _, err := c.SystemOne(t.Context(), req, jev.WithHeaders(http.Header{"X-Expected": {"default"}, "X-Model": {"override"}})); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListModels(t.Context(), option); err != nil {
		t.Fatal(err)
	}
}

func TestPerCallTimeoutDoesNotMutateHTTPClient(t *testing.T) {
	h := &http.Client{Timeout: time.Hour, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	c, err := jev.NewClient(jev.Config{APIKey: "key", HTTPClient: h})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	started := time.Now()
	if _, err := c.ListModels(ctx, jev.WithTimeout(time.Millisecond)); err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("per-call timeout not applied")
	}
	if h.Timeout != time.Hour {
		t.Fatal("mutated injected HTTP client")
	}
	for _, d := range []time.Duration{0, -1} {
		if _, err := c.ListModels(ctx, jev.WithTimeout(d)); err == nil {
			t.Fatal("accepted invalid timeout")
		}
	}
	if _, err := jev.NewClient(jev.Config{APIKey: "key", Timeout: -1}); err == nil {
		t.Fatal("accepted negative timeout")
	}
}
