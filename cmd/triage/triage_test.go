package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jev "github.com/withzombies/jev-go"
)

type evaluatorFunc func(context.Context, jev.Request) (*jev.Response, error)

func (f evaluatorFunc) SystemOne(ctx context.Context, r jev.Request) (*jev.Response, error) {
	return f(ctx, r)
}

// Complete fixture covering every question in the editable catalog.
func answersFor(questions []reviewQuestion) map[string]jev.Answer {
	answers := make(map[string]jev.Answer, len(questions))
	for _, q := range questions {
		switch typed := q.Question.(type) {
		case jev.Noul:
			answers[q.ID] = jev.NoulAnswer{Noul: 0}
		case jev.Choice:
			probabilities := make(map[string]float64)
			for label := range typed.Criteria {
				probabilities[label] = 0
			}
			probabilities["bugfix"] = 1
			answers[q.ID] = jev.ChoiceAnswer{Choice: "bugfix", Confidence: 1, Probabilities: probabilities}
		case jev.Score:
			legend := make(map[string]any)
			probabilities := make(map[string]float64)
			for level, description := range typed.Criteria {
				key := fmt.Sprint(level)
				legend[key] = description
				probabilities[key] = 0
			}
			probabilities["0"] = .75
			probabilities["1"] = .25
			answers[q.ID] = jev.ScoreAnswer{Score: .25, Confidence: .6, Probabilities: probabilities, Legend: legend}
		}
	}
	answers["context_sufficient"] = jev.NoulAnswer{Noul: .9}
	return answers
}

func firstBlocker(questions []reviewQuestion) string {
	for _, q := range questions {
		if q.Blocker {
			return q.ID
		}
	}
	panic("catalog has no blockers")
}

func TestAssessVerdictBoundaries(t *testing.T) {
	questions := reviewQuestions()
	blocker := firstBlocker(questions)
	for _, tc := range []struct {
		name          string
		risk, context float64
		want          string
	}{
		{"approve", 0, .9, "approve"}, {"inclusive approval", .2, .8, "approve"},
		{"uncertain risk", math.Nextafter(.2, 1), .9, "needs_human_review"},
		{"below blocker", math.Nextafter(.8, 0), .9, "needs_human_review"},
		{"inclusive blocker", .8, .9, "request_changes"},
		{"context insufficient", 0, math.Nextafter(.8, 0), "needs_human_review"},
		{"blocker with uncertain context", .95, .3, "request_changes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answers := answersFor(questions)
			answers[blocker] = jev.NoulAnswer{Noul: tc.risk}
			answers["context_sufficient"] = jev.NoulAnswer{Noul: tc.context}
			verdict, reasons, err := assess(answers, questions)
			if err != nil {
				t.Fatal(err)
			}
			if verdict != tc.want || len(reasons) == 0 {
				t.Fatalf("got %s %v, want %s", verdict, reasons, tc.want)
			}
		})
	}
}

func TestAssessRejectsMissingOrInvalidEvidence(t *testing.T) {
	questions := reviewQuestions()
	blocker := firstBlocker(questions)
	for _, tc := range []struct {
		name, id string
		answer   jev.Answer
	}{
		{"missing blocker", blocker, nil}, {"missing context", "context_sufficient", nil},
		{"wrong answer type", blocker, jev.ChoiceAnswer{Choice: "yes"}},
		{"negative", blocker, jev.NoulAnswer{Noul: -.1}}, {"above one", blocker, jev.NoulAnswer{Noul: 1.1}},
		{"nan", blocker, jev.NoulAnswer{Noul: math.NaN()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answers := answersFor(questions)
			answers[tc.id] = tc.answer
			if _, _, err := assess(answers, questions); err == nil {
				t.Fatal("accepted invalid evidence")
			}
		})
	}
}

func TestRunPassesStdinAndOptionsToInjectedEvaluator(t *testing.T) {
	patch := "diff --git a/file.go b/file.go\n-old\n+new\n"
	questions := reviewQuestions()
	var calls int
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "propagated")
	evaluate := evaluatorFunc(func(gotCtx context.Context, req jev.Request) (*jev.Response, error) {
		calls++
		if gotCtx.Value(contextKey{}) != "propagated" {
			t.Error("context not forwarded")
		}
		if req.State != patch || req.Model != "pinned" {
			t.Fatalf("request: %#v", req)
		}
		if len(req.Questions) != len(questions) {
			t.Fatal("questions were not batched")
		}
		return &jev.Response{Model: "pinned", Usage: jev.Usage{InputTokens: 10, OutputTokens: 3}, RequestID: "req-example", Answers: answersFor(questions)}, nil
	})
	var out bytes.Buffer
	if err := run(ctx, evaluate, strings.NewReader(patch), &out, options{model: "pinned", jsonOutput: true}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("evaluation calls: %d", calls)
	}
	// Decode just the report envelope here; answers retain their wire tags.
	var wire struct {
		Verdict   string            `json:"verdict"`
		Model     string            `json:"model"`
		RequestID string            `json:"request_id"`
		Usage     jev.Usage         `json:"usage"`
		Results   []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Verdict != "approve" || wire.Model != "pinned" || wire.RequestID != "req-example" || wire.Usage.InputTokens != 10 || len(wire.Results) != len(questions) {
		t.Fatalf("report: %s", out.String())
	}
	if strings.Contains(out.String(), patch) {
		t.Fatal("report unnecessarily repeats input patch")
	}
}

func TestRunRealClientEndToEnd(t *testing.T) {
	questions := reviewQuestions()
	answers := answersFor(questions)
	answers[firstBlocker(questions)] = jev.NoulAnswer{Noul: .95}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State     string                     `json:"state"`
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.State != "sample patch" || len(body.Questions) != len(questions) {
			t.Error("incorrect evaluation payload")
		}
		w.Header().Set("x-typesafe-request-id", "req-integration")
		if err := json.NewEncoder(w).Encode(jev.Response{Model: "jev-test", Usage: jev.Usage{InputTokens: 123, OutputTokens: 45}, Answers: answers}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := jev.NewClient(jev.Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), client, strings.NewReader("sample patch"), &out, options{}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Verdict: request_changes", "0.95", "jev-test", "123", "45", "req-integration", firstBlocker(questions)} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, out.String())
		}
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRunInputFailuresDoNotEvaluate(t *testing.T) {
	want := errors.New("input broke")
	for _, in := range []io.Reader{strings.NewReader(""), strings.NewReader(" \n\t"), failingReader{want}} {
		evaluate := evaluatorFunc(func(context.Context, jev.Request) (*jev.Response, error) {
			t.Error("unexpected evaluation")
			return nil, nil
		})
		err := run(context.Background(), evaluate, in, io.Discard, options{})
		if err == nil {
			t.Fatal("accepted invalid input")
		}
		if _, ok := in.(failingReader); ok && !errors.Is(err, want) {
			t.Fatalf("lost cause: %v", err)
		}
	}
}

func TestRunPreservesEvaluatorAndWriterErrors(t *testing.T) {
	want := errors.New("dependency failed")
	failed := evaluatorFunc(func(context.Context, jev.Request) (*jev.Response, error) { return nil, want })
	var out bytes.Buffer
	if err := run(context.Background(), failed, strings.NewReader("patch"), &out, options{}); !errors.Is(err, want) {
		t.Fatalf("lost evaluator error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("printed recommendation on API failure")
	}
	good := evaluatorFunc(func(context.Context, jev.Request) (*jev.Response, error) {
		return &jev.Response{Model: "jev", Answers: answersFor(reviewQuestions())}, nil
	})
	for _, jsonOutput := range []bool{false, true} {
		if err := run(context.Background(), good, strings.NewReader("patch"), failingWriter{want}, options{jsonOutput: jsonOutput}); !errors.Is(err, want) {
			t.Fatalf("lost writer error: %v", err)
		}
	}
}

func TestRunRejectsIncompleteEvaluatorResponse(t *testing.T) {
	for _, response := range []*jev.Response{nil, {Model: "jev", Answers: map[string]jev.Answer{}}} {
		evaluate := evaluatorFunc(func(context.Context, jev.Request) (*jev.Response, error) { return response, nil })
		if err := run(context.Background(), evaluate, strings.NewReader("patch"), io.Discard, options{}); err == nil {
			t.Fatal("accepted incomplete response")
		}
	}
}

func TestCommandFlagsAndEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		key      string
		wantCode int
		message  string
	}{
		{"help", []string{"--help"}, "", 0, "Usage"},
		{"unknown flag", []string{"--unknown"}, "", 1, "flag provided"},
		{"positional arguments", []string{"8556"}, "", 1, "stdin"},
		{"missing key", nil, "", 1, "TYPESAFE_API_KEY"},
		{"empty input", []string{"--json", "--model", "pinned"}, "dummy-key", 1, "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			getenv := func(name string) string {
				if name != "TYPESAFE_API_KEY" {
					t.Errorf("unexpected env var %s", name)
				}
				return tc.key
			}
			code := command(context.Background(), tc.args, io.NopCloser(strings.NewReader("")), &out, &stderr, getenv)
			if code != tc.wantCode || !strings.Contains(stderr.String(), tc.message) {
				t.Fatalf("code=%d stderr=%s", code, stderr.String())
			}
			if out.Len() != 0 {
				t.Fatal("unexpected report")
			}
		})
	}
}

func TestQuestionCatalogHasUniqueIDsAndMixedTypes(t *testing.T) {
	seen := map[string]bool{}
	types := map[string]bool{}
	for _, q := range reviewQuestions() {
		if q.ID == "" || q.Label == "" || seen[q.ID] {
			t.Fatalf("invalid catalog entry: %#v", q)
		}
		seen[q.ID] = true
		raw, err := json.Marshal(q.Question)
		if err != nil {
			t.Fatal(err)
		}
		var wire struct {
			Type         string `json:"type"`
			Instructions string `json:"instructions"`
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		if wire.Instructions == "" {
			t.Errorf("missing instructions for %s", q.ID)
		}
		types[wire.Type] = true
		if q.Blocker && wire.Type != "noul" {
			t.Errorf("blocker %s is not a probability", q.ID)
		}
	}
	if len(types) != 3 || !seen["context_sufficient"] {
		t.Fatal(fmt.Sprint("catalog lacks mixed types or context question: ", types))
	}
}

func TestCommandCancellationUnblocksInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, writer := io.Pipe()
	defer func() {
		if err := input.Close(); err != nil {
			t.Error(err)
		}
	}()
	defer func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	}()
	var out, stderr bytes.Buffer
	started := make(chan struct{})
	done := make(chan int, 1)
	go func() {
		done <- command(ctx, nil, input, &out, &stderr, func(string) string {
			close(started)
			return "dummy-key"
		})
	}()
	<-started
	cancel()
	select {
	case code := <-done:
		if code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "read stdin") {
			t.Fatalf("code=%d out=%s stderr=%s", code, out.String(), stderr.String())
		}
	case <-time.After(time.Second):
		t.Fatal("command remained blocked on stdin after cancellation")
	}
}

func TestRunInputPrefix(t *testing.T) {
	for _, tc := range []struct {
		name, input, prefix string
		limit               int64
	}{
		{"short", "patch", "patch", 10},
		{"exact", "patch", "patch", 5},
		{"long", "patch remainder", "patch", 5},
		{"unicode split", "a☃z", "a", 3},
		{"unicode exact", "a☃z", "a☃", 4},
		{"default", strings.Repeat("x", 24577), strings.Repeat("x", 24576), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, asJSON := range []bool{false, true} {
				input := strings.NewReader(tc.input)
				calls := 0
				evaluate := evaluatorFunc(func(_ context.Context, r jev.Request) (*jev.Response, error) {
					calls++
					if r.State != tc.prefix {
						t.Errorf("state differs from expected prefix")
					}
					if input.Len() != 0 {
						t.Error("stdin not drained before evaluation")
					}
					return &jev.Response{Model: "test", Answers: answersFor(reviewQuestions())}, nil
				})
				var out bytes.Buffer
				if err := run(context.Background(), evaluate, input, &out, options{contextBytes: tc.limit, jsonOutput: asJSON}); err != nil {
					t.Fatal(err)
				}
				if calls != 1 {
					t.Fatalf("evaluation calls: %d", calls)
				}
				truncated := len(tc.prefix) < len(tc.input)
				if asJSON {
					var got struct {
						Verdict        string `json:"verdict"`
						InputBytes     int64  `json:"input_bytes"`
						EvaluatedBytes int    `json:"evaluated_bytes"`
						Truncated      bool   `json:"truncated"`
					}
					if err := json.Unmarshal(out.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if got.Verdict != "approve" || got.InputBytes != int64(len(tc.input)) || got.EvaluatedBytes != len(tc.prefix) || got.Truncated != truncated {
						t.Fatalf("report: %s", out.String())
					}
				} else {
					if !strings.Contains(out.String(), fmt.Sprintf("%d of %d bytes", len(tc.prefix), len(tc.input))) {
						t.Fatalf("missing byte counts: %s", out.String())
					}
					if truncated && !strings.Contains(out.String(), "input prefix only") {
						t.Fatalf("missing partial scope: %s", out.String())
					}
				}
			}
		})
	}
}

func TestRunDrainFailureDoesNotEvaluate(t *testing.T) {
	want := errors.New("drain failed")
	input := io.MultiReader(strings.NewReader("patch"), failingReader{want})
	eval := evaluatorFunc(func(context.Context, jev.Request) (*jev.Response, error) {
		t.Error("unexpected evaluation")
		return nil, nil
	})
	if err := run(context.Background(), eval, input, io.Discard, options{contextBytes: 5}); !errors.Is(err, want) {
		t.Fatalf("lost drain error: %v", err)
	}
}

func TestCommandContextBytes(t *testing.T) {
	for _, value := range []string{"0", "-1", "invalid"} {
		var stderr bytes.Buffer
		code := command(context.Background(), []string{"--context-bytes", value}, io.NopCloser(strings.NewReader("patch")), io.Discard, &stderr, func(string) string { return "test" })
		if code != 1 || !strings.Contains(stderr.String(), "context-bytes") {
			t.Fatalf("code=%d error=%s", code, stderr.String())
		}
	}
	var stderr bytes.Buffer
	command(context.Background(), []string{"--help"}, io.NopCloser(strings.NewReader("")), io.Discard, &stderr, func(string) string { return "" })
	if !strings.Contains(stderr.String(), "context-bytes") || !strings.Contains(stderr.String(), "24576") {
		t.Fatalf("help: %s", stderr.String())
	}
}

func TestCommandCancellationUnblocksDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, writer := io.Pipe()
	defer func() {
		if err := input.Close(); err != nil {
			t.Error(err)
		}
	}()
	defer func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	}()
	done := make(chan int, 1)
	var stderr bytes.Buffer
	go func() {
		done <- command(ctx, []string{"--context-bytes", "1"}, input, io.Discard, &stderr, func(string) string { return "test" })
	}()
	written := make(chan error, 1)
	go func() { _, err := io.WriteString(writer, "xy"); written <- err }()
	select {
	case err := <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("input not consumed")
	}
	cancel()
	select {
	case code := <-done:
		if code != 1 || !strings.Contains(stderr.String(), "read stdin") {
			t.Fatalf("code=%d stderr=%s", code, stderr.String())
		}
	case <-time.After(time.Second):
		t.Fatal("blocked draining after cancellation")
	}
}

func TestRunContextLimitGuidance(t *testing.T) {
	want := &jev.APIError{StatusCode: 400, ErrorType: "max_tokens_exceeded", RequestID: "req-limit"}
	eval := evaluatorFunc(func(context.Context, jev.Request) (*jev.Response, error) { return nil, want })
	err := run(context.Background(), eval, strings.NewReader("patch"), io.Discard, options{})
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "--context-bytes") {
		t.Fatalf("error: %v", err)
	}
}
