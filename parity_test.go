package jev_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	jev "github.com/withzombies/jev-go"
)

func TestEffectiveRequestAndRawQuestions(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if string(body["state"]) != `{"new":true}` || string(body["model"]) != `"extra-model"` || string(body["extension"]) != "null" {
			t.Errorf("wrong merged body: %s", body)
		}
		if strings.Contains(string(body["questions"]), "original") {
			t.Error("questions were deep merged")
		}
		w.Header().Set("x-typesafe-request-id", "req-raw")
		writeResponse(t, w, `{"model":"extra-model","usage":{},"answers":{"future":{"type":"future","value":{"custom":true}}}}`)
	})
	req := jev.Request{State: map[string]any{"old": true}, Questions: map[string]jev.Question{"original": jev.Noul{}}, ExtraBody: map[string]any{
		"state": map[string]any{"new": true}, "model": "extra-model", "extension": nil,
		"questions": map[string]jev.Question{"future": jev.RawQuestion{"type": "future", "extra": nil}},
	}}
	response, err := c.SystemOne(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := response.Answers["future"].(jev.RawAnswer)
	if !ok || raw.Type != "future" || !strings.Contains(string(raw.Body), `"custom":true`) {
		t.Fatalf("raw answer: %#v", response.Answers)
	}
	if response.HTTPResponse.StatusCode != 200 || response.HTTPResponse.Headers.Get("x-typesafe-request-id") != "req-raw" || !strings.Contains(string(response.HTTPResponse.Body), `"future"`) {
		t.Fatal("missing HTTP metadata")
	}
	if _, ok := req.State.(map[string]any)["old"]; !ok {
		t.Fatal("request mutated")
	}
	wire, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "HTTPResponse") || strings.Contains(string(wire), "Headers") {
		t.Fatal("transport metadata serialized")
	}
	var again jev.Response
	if err := json.Unmarshal(wire, &again); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.Answers, response.Answers) {
		t.Fatal("raw answer changed on round trip")
	}
}

func TestExtraBodyByteLimit(t *testing.T) {
	req := oneRequest()
	req.Model = jev.DefaultModel
	req.ExtraBody = map[string]any{"additional": strings.Repeat("x", 200)}
	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{len(encoded), len(encoded) - 1} {
		calls := 0
		c, err := jev.NewClient(jev.Config{APIKey: "key", MaxRequestBytes: limit, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			if string(body) != string(encoded) {
				t.Error("wrong effective request bytes")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(oneResponse))}, nil
		})}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.SystemOne(t.Context(), req)
		if limit == len(encoded) {
			if err != nil || calls != 1 {
				t.Fatalf("exact limit failed: %v", err)
			}
		} else {
			var size *jev.RequestSizeError
			if !errors.As(err, &size) || size.Size != len(encoded) || calls != 0 {
				t.Fatalf("oversize not rejected before HTTP: %v", err)
			}
		}
	}
}

func TestQuestionValidationUsesEffectivePayload(t *testing.T) {
	c := clientFor(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid payload reached HTTP") })
	for _, q := range []jev.Question{jev.RawQuestion{}, jev.RawQuestion{"type": ""}, jev.RawQuestion{"type": 1}, jev.Score{}, jev.Choice{}, jev.RawQuestion{"type": "score", "criteria": []any{}}, jev.RawQuestion{"type": "choice", "criteria": []any{"bad"}}} {
		req := oneRequest()
		req.Questions = map[string]jev.Question{"bad": q}
		if _, err := c.SystemOne(t.Context(), req); err == nil {
			t.Errorf("accepted %T", q)
		}
	}
	for _, value := range []any{nil, map[string]any{}, "wrong"} {
		req := oneRequest()
		req.ExtraBody = map[string]any{"questions": value}
		if _, err := c.SystemOne(t.Context(), req); err == nil {
			t.Fatal("accepted invalid effective questions")
		}
	}
}

func TestSingleScoreCriterionAndExplicitNull(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if !strings.Contains(string(body), `"instructions":null`) {
			t.Error("explicit null was lost")
		}
		writeResponse(t, w, `{"model":"jev","usage":{},"answers":{"score":{"type":"score","score":0,"confidence":1,"probabilities":{"0":1},"legend":{"0":"only"}}}}`)
	})
	req := jev.Request{State: "x", Questions: map[string]jev.Question{"score": jev.RawQuestion{"type": "score", "instructions": nil, "criteria": []any{"only"}}}}
	if _, err := c.SystemOne(t.Context(), req); err != nil {
		t.Fatal(err)
	}
}

func TestRawEndpointsSkipDecoding(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-typesafe-request-id", "req-opaque")
		w.WriteHeader(201)
		writeResponse(t, w, "opaque")
	})
	evaluation, err := c.SystemOneRaw(t.Context(), oneRequest())
	if err != nil {
		t.Fatal(err)
	}
	models, err := c.ListModelsRaw(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []*jev.HTTPResponse{evaluation, models} {
		if r.StatusCode != 201 || r.RequestID != "req-opaque" || string(r.Body) != "opaque" {
			t.Fatalf("wrong raw response: %#v", r)
		}
	}
	c = clientFor(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401); writeResponse(t, w, "denied") })
	_, err = c.ListModelsRaw(t.Context())
	var api *jev.APIError
	if !errors.As(err, &api) {
		t.Fatalf("raw bypassed status errors: %v", err)
	}
}

func TestModelListMetadata(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-typesafe-request-id", "models-id")
		writeResponse(t, w, `{"models":[{"name":"jev","description":"model","release_date":"today","future":true}]}`)
	})
	r, err := c.ListModels(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Models) != 1 || r.Models[0].Name != "jev" || r.RequestID != "models-id" || r.HTTPResponse.StatusCode != 200 {
		t.Fatalf("metadata: %#v", r)
	}
}

func TestAnswerGroupsAndOptionalUsage(t *testing.T) {
	var response jev.Response
	body := strings.Replace(mixedResponse, `"input_tokens":12,"output_tokens":8`, `"input_tokens":null`, 1)
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	if response.Usage.InputTokens != nil || response.Usage.OutputTokens != nil {
		t.Fatal("unavailable usage became zero")
	}
	if len(response.Nouls()) != 1 || response.Nouls()["urgent"].Noul != 0 || response.Choices()["category"].Choice != "bug" || response.Scores()["severity"].Score != .25 {
		t.Fatal("wrong answer groups")
	}
	group := response.Nouls()
	delete(group, "urgent")
	if len(response.Nouls()) != 1 {
		t.Fatal("group map mutated source")
	}
}

func TestResponseValidationDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		body, path string
		models     bool
	}{
		{`{"model":"jev","usage":{},"answers":{"ok":{"type":"noul","noul":null}}}`, "answers.ok.noul", false},
		{`{"model":"jev","usage":{},"answers":{"ok":{"type":"choice","choice":"x","probabilities":{"x":1}}}}`, "answers.ok.confidence", false},
		{`{"model":"jev","usage":{},"answers":{}}`, "answers.ok", false},
		{`{"model":"jev","usage":{"input_tokens":"bad"},"answers":{}}`, "usage.input_tokens", false},
		{`{"models":[{"name":"jev"}]}`, "models.0.description", true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("x-typesafe-request-id", "invalid-id")
				writeResponse(t, w, tc.body)
			})
			var err error
			if tc.models {
				_, err = c.ListModels(t.Context())
			} else {
				_, err = c.SystemOne(t.Context(), oneRequest())
			}
			var invalid *jev.ResponseValidationError
			if !errors.As(err, &invalid) || invalid.FieldPath != tc.path || invalid.HTTPResponse.RequestID != "invalid-id" || string(invalid.HTTPResponse.Body) != tc.body || errors.Unwrap(invalid) == nil {
				t.Fatalf("validation diagnostics: %#v (%v)", invalid, err)
			}
		})
	}
}

func TestAPIErrorDiagnostics(t *testing.T) {
	for _, tc := range []struct{ body, message string }{{`{"detail":{"error_type":"bad","message":"details"}}`, "details"}, {`{"error":{"message":"nested"}}`, "nested"}, {`{"detail":[{"loc":["body","questions","x"],"msg":"invalid"}]}`, "questions.x: invalid"}, {"plain failure", "plain failure"}, {"", ""}} {
		c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After-Ms", "250")
			w.WriteHeader(429)
			writeResponse(t, w, tc.body)
		})
		_, err := c.ListModels(context.Background())
		var api *jev.APIError
		if !errors.As(err, &api) || api.Message != tc.message || api.Endpoint != "GET /v1/models" || api.RetryAfter == nil || *api.RetryAfter != 250*time.Millisecond {
			t.Fatalf("diagnostics: %#v", api)
		}
		if tc.message != "" && strings.Contains(api.Error(), tc.message) {
			t.Fatal("Error() exposed server content")
		}
	}
}

func TestAdditionalRetryCanHandleResponseValidation(t *testing.T) {
	calls := 0
	policy := jev.RetryPolicy{MaxRetries: 1, AdditionalRetry: func(err error) bool { var invalid *jev.ResponseValidationError; return errors.As(err, &invalid) }}
	c, err := jev.NewClient(jev.Config{APIKey: "key", Retry: policy, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		body := "invalid"
		if calls == 2 {
			body = oneResponse
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SystemOne(t.Context(), oneRequest()); err != nil {
		t.Fatalf("validation retry failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestValidationErrorWithoutMetadataIsPrintable(t *testing.T) {
	err := &jev.ResponseValidationError{FieldPath: "model", Err: io.ErrUnexpectedEOF}
	if !strings.Contains(err.Error(), "model") {
		t.Fatal("missing field path")
	}
}
