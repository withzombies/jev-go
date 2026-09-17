package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	jev "github.com/withzombies/jev-go"
)

// Consumers define the interface they need; *jev.Client satisfies this directly.
type evaluator interface {
	SystemOne(context.Context, jev.Request, ...jev.RequestOption) (*jev.Response, error)
}

const defaultContextBytes int64 = 24 * 1024

type options struct {
	contextBytes int64
	model        string
	jsonOutput   bool
}

type questionResult struct {
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Answer jev.Answer `json:"answer"`
}

type report struct {
	InputBytes     int64            `json:"input_bytes"`
	EvaluatedBytes int              `json:"evaluated_bytes"`
	Truncated      bool             `json:"truncated"`
	Verdict        string           `json:"verdict"`
	Reasons        []string         `json:"reasons"`
	Model          string           `json:"model"`
	Usage          jev.Usage        `json:"usage"`
	RequestID      string           `json:"request_id,omitempty"`
	Results        []questionResult `json:"results"`
}

func run(ctx context.Context, eval evaluator, in io.Reader, out io.Writer, opts options) error {
	limit := opts.contextBytes
	if limit == 0 {
		limit = defaultContextBytes
	}
	if limit < 0 {
		return fmt.Errorf("context-bytes must be positive")
	}
	patch, err := io.ReadAll(io.LimitReader(in, limit))
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	// Consume the rest without retaining it, allowing pipeline producers to finish.
	remaining, err := io.Copy(io.Discard, in)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	inputBytes := int64(len(patch)) + remaining
	if remaining > 0 && len(patch) > 0 {
		start := len(patch) - 1
		for start > 0 && !utf8.RuneStart(patch[start]) {
			start--
		}
		if !utf8.FullRune(patch[start:]) {
			patch = patch[:start]
		}
	}
	if strings.TrimSpace(string(patch)) == "" {
		return fmt.Errorf("stdin is empty; pipe a PR diff or patch into triage")
	}
	questions := reviewQuestions()
	request := jev.Request{State: string(patch), Model: opts.model, Questions: make(map[string]jev.Question, len(questions))}
	for _, q := range questions {
		request.Questions[q.ID] = q.Question
	}
	response, err := eval.SystemOne(ctx, request)
	if err != nil {
		var apiErr *jev.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorType == "max_tokens_exceeded" {
			return fmt.Errorf("evaluate patch: reduce --context-bytes to leave room for the questions: %w", err)
		}
		return fmt.Errorf("evaluate patch: %w", err)
	}
	if response == nil {
		return fmt.Errorf("evaluator returned no response")
	}
	verdict, reasons, err := assess(response.Answers, questions)
	if err != nil {
		return err
	}
	result := report{InputBytes: inputBytes, EvaluatedBytes: len(patch), Truncated: int64(len(patch)) < inputBytes, Verdict: verdict, Reasons: reasons, Model: response.Model, Usage: response.Usage, RequestID: response.RequestID}
	for _, q := range questions {
		answer, ok := response.Answers[q.ID]
		if !ok || answer == nil {
			return fmt.Errorf("missing answer %q", q.ID)
		}
		result.Results = append(result.Results, questionResult{ID: q.ID, Label: q.Label, Answer: answer})
	}
	if opts.jsonOutput {
		if err := json.NewEncoder(out).Encode(result); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		return nil
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Verdict: %s\n", result.Verdict)
	if result.Truncated {
		fmt.Fprintln(&text, "Scope: input prefix only; remaining input was not evaluated.")
	}
	fmt.Fprintf(&text, "Evaluated: %d of %d bytes\n", result.EvaluatedBytes, result.InputBytes)
	for _, reason := range result.Reasons {
		fmt.Fprintf(&text, "- %s\n", reason)
	}
	fmt.Fprintf(&text, "\nModel: %s | Tokens: %d input, %d output\n", result.Model, result.Usage.InputTokens, result.Usage.OutputTokens)
	if result.RequestID != "" {
		fmt.Fprintf(&text, "Request: %s\n", result.RequestID)
	}
	for _, r := range result.Results {
		answer, err := json.Marshal(r.Answer)
		if err != nil {
			return fmt.Errorf("encode answer %q: %w", r.ID, err)
		}
		fmt.Fprintf(&text, "\n%s: %s\n  %s\n", r.ID, r.Label, answer)
	}
	if _, err := io.WriteString(out, text.String()); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// assess is deliberately plain, provisional application policy. These thresholds
// are experiment defaults, not calibrated measures of review accuracy.
func assess(answers map[string]jev.Answer, questions []reviewQuestion) (string, []string, error) {
	contextProbability, err := noulProbability(answers, "context_sufficient")
	if err != nil {
		return "", nil, err
	}
	var blockers, uncertain []string
	for _, q := range questions {
		if !q.Blocker {
			continue
		}
		p, err := noulProbability(answers, q.ID)
		if err != nil {
			return "", nil, err
		}
		reason := fmt.Sprintf("%s: %s (probability %g)", q.ID, q.Label, p)
		if p >= .8 {
			blockers = append(blockers, reason)
		} else if p > .2 {
			uncertain = append(uncertain, reason)
		}
	}
	if contextProbability < .8 {
		uncertain = append(uncertain, fmt.Sprintf("Insufficient review context (probability of sufficient context %g)", contextProbability))
	}
	if len(blockers) > 0 {
		return "request_changes", append(blockers, uncertain...), nil
	}
	if len(uncertain) > 0 {
		return "needs_human_review", uncertain, nil
	}
	return "approve", []string{"All blocker probabilities are at most 0.2; context sufficiency is at least 0.8."}, nil
}

func noulProbability(answers map[string]jev.Answer, id string) (float64, error) {
	a, ok := answers[id].(jev.NoulAnswer)
	if !ok {
		return 0, fmt.Errorf("missing or incorrect Noul answer %q", id)
	}
	if math.IsNaN(a.Noul) || a.Noul < 0 || a.Noul > 1 {
		return 0, fmt.Errorf("invalid probability for %q", id)
	}
	return a.Noul, nil
}
