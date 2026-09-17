package jev

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Question is one of Noul, Choice, or Score. Instructions and descriptions may
// contain text, JSON objects, or arrays. See the API for supported content.
type Question interface {
	json.Marshaler
	questionType() string
}

// Noul asks for the probability that a statement is true.
type Noul struct {
	// Instructions describes the judgment to make against the request state.
	Instructions any `json:"instructions,omitempty"`
	// Criteria optionally describes true and false outcomes; nil omits it.
	Criteria *NoulCriteria `json:"criteria,omitempty"`
}

// NoulCriteria optionally describes the two outcomes.
type NoulCriteria struct {
	// True describes evidence for the statement; nil omits it.
	True any `json:"true,omitempty"`
	// False describes evidence against the statement; nil omits it.
	False any `json:"false,omitempty"`
}

// Choice selects a label. A nil criterion leaves its label undescribed.
type Choice struct {
	// Instructions describes the judgment to make against the request state.
	Instructions any `json:"instructions,omitempty"`
	// Criteria maps allowed labels to descriptions. A nil value leaves a label undescribed.
	Criteria map[string]any `json:"criteria"`
}

// Score evaluates an ordered rubric whose levels start at zero.
type Score struct {
	// Instructions describes the judgment to make against the request state.
	Instructions any `json:"instructions,omitempty"`
	// Criteria lists level descriptions in increasing order, starting at score zero.
	Criteria []any `json:"criteria"`
}

func (Noul) questionType() string   { return "noul" }
func (Choice) questionType() string { return "choice" }
func (Score) questionType() string  { return "score" }

// MarshalJSON encodes Noul with its API type discriminator.
func (q Noul) MarshalJSON() ([]byte, error) {
	type fields Noul
	return json.Marshal(struct {
		Type string `json:"type"`
		fields
	}{"noul", fields(q)})
}

// MarshalJSON encodes Choice with its API type discriminator.
func (q Choice) MarshalJSON() ([]byte, error) {
	type fields Choice
	return json.Marshal(struct {
		Type string `json:"type"`
		fields
	}{"choice", fields(q)})
}

// MarshalJSON encodes Score with its API type discriminator.
func (q Score) MarshalJSON() ([]byte, error) {
	type fields Score
	return json.Marshal(struct {
		Type string `json:"type"`
		fields
	}{"score", fields(q)})
}

// Request evaluates State against independently answered named Questions.
// State must encode as a JSON string, object, or array. Empty Model uses DefaultModel.
type Request struct {
	// State is the shared JSON-encodable context for all questions.
	State any `json:"state"`
	// Questions maps caller-chosen identifiers to non-nil question values.
	Questions map[string]Question `json:"questions"`
	// Model selects a model name or alias; empty uses DefaultModel.
	Model string `json:"model"`
}

// Answer is a NoulAnswer, ChoiceAnswer, or ScoreAnswer, decoded by its wire type.
type Answer interface {
	json.Marshaler
	answerType() string
}

// NoulAnswer holds the probability of yes; it has no separate confidence field.
type NoulAnswer struct {
	// Noul is the probability of true, from zero to one inclusive.
	Noul float64 `json:"noul"`
}

// ChoiceAnswer contains the selected label and the complete distribution.
type ChoiceAnswer struct {
	// Choice is the selected label.
	Choice string `json:"choice"`
	// Confidence is the service-provided confidence value, from zero to one.
	Confidence float64 `json:"confidence"`
	// Probabilities maps each allowed label to its probability.
	Probabilities map[string]float64 `json:"probabilities"`
}

// ScoreAnswer contains an expected, possibly fractional score. Map keys are
// zero-based rubric levels as strings, matching the API's JSON representation.
type ScoreAnswer struct {
	// Score is the expected score and may lie between rubric levels.
	Score float64 `json:"score"`
	// Confidence is the service-provided confidence value, from zero to one.
	Confidence float64 `json:"confidence"`
	// Probabilities maps each zero-based rubric level, encoded as a string, to its probability.
	Probabilities map[string]float64 `json:"probabilities"`
	// Legend maps zero-based rubric levels, encoded as strings, to descriptions.
	Legend map[string]any `json:"legend"`
}

func (NoulAnswer) answerType() string   { return "noul" }
func (ChoiceAnswer) answerType() string { return "choice" }
func (ScoreAnswer) answerType() string  { return "score" }

// MarshalJSON encodes NoulAnswer with its API type discriminator.
func (a NoulAnswer) MarshalJSON() ([]byte, error) {
	type fields NoulAnswer
	return json.Marshal(struct {
		Type string `json:"type"`
		fields
	}{"noul", fields(a)})
}

// MarshalJSON encodes ChoiceAnswer with its API type discriminator.
func (a ChoiceAnswer) MarshalJSON() ([]byte, error) {
	type fields ChoiceAnswer
	return json.Marshal(struct {
		Type string `json:"type"`
		fields
	}{"choice", fields(a)})
}

// MarshalJSON encodes ScoreAnswer with its API type discriminator.
func (a ScoreAnswer) MarshalJSON() ([]byte, error) {
	type fields ScoreAnswer
	return json.Marshal(struct {
		Type string `json:"type"`
		fields
	}{"score", fields(a)})
}

// Usage contains the token counts returned by the service.
type Usage struct {
	// InputTokens is the input token count reported by the service.
	InputTokens int `json:"input_tokens"`
	// OutputTokens is the output token count reported by the service.
	OutputTokens int `json:"output_tokens"`
}

// Response contains one typed answer per question and server metadata.
type Response struct {
	// Model is the resolved model used for evaluation.
	Model string `json:"model"`
	// Answers maps question identifiers to typed answer values.
	Answers map[string]Answer `json:"answers"`
	// Usage contains the token counts returned by the service.
	Usage Usage `json:"usage"`
	// RequestID is the x-typesafe-request-id response header, if present.
	RequestID string `json:"request_id,omitempty"`
}

// Model describes a model or alias accepted by the service.
type Model struct {
	// Name is the model identifier accepted by Request.Model.
	Name string `json:"name"`
	// Description is the service-provided model description.
	Description string `json:"description"`
	// ReleaseDate preserves the service-provided release date string.
	ReleaseDate string `json:"release_date"`
}

// UnmarshalJSON decodes the API's discriminated answer objects. Unknown types
// and missing required fields are errors, never implicit zero-valued answers.
func (r *Response) UnmarshalJSON(data []byte) error {
	fields, err := requiredFields(data, "model", "answers", "usage")
	if err != nil {
		return err
	}
	if _, err := requiredFields(fields["usage"], "input_tokens", "output_tokens"); err != nil {
		return fmt.Errorf("usage: %w", err)
	}
	var wire struct {
		Model     string                     `json:"model"`
		Answers   map[string]json.RawMessage `json:"answers"`
		Usage     Usage                      `json:"usage"`
		RequestID string                     `json:"request_id"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Model == "" {
		return fmt.Errorf("model is empty")
	}
	answers := make(map[string]Answer, len(wire.Answers))
	for name, raw := range wire.Answers {
		answer, err := decodeAnswer(raw)
		if err != nil {
			return fmt.Errorf("answer %q: %w", name, err)
		}
		answers[name] = answer
	}
	*r = Response{Model: wire.Model, Answers: answers, Usage: wire.Usage, RequestID: wire.RequestID}
	return nil
}

func requiredFields(data []byte, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for _, name := range names {
		value := bytes.TrimSpace(fields[name])
		if len(value) == 0 || bytes.Equal(value, []byte("null")) {
			return nil, fmt.Errorf("missing or null %s", name)
		}
	}
	return fields, nil
}

func decodeAnswer(raw []byte) (Answer, error) {
	fields, err := requiredFields(raw, "type")
	if err != nil {
		return nil, err
	}
	var kind string
	if err := json.Unmarshal(fields["type"], &kind); err != nil {
		return nil, err
	}
	switch kind {
	case "noul":
		if _, err := requiredFields(raw, "noul"); err != nil {
			return nil, err
		}
		var a NoulAnswer
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		if a.Noul < 0 || a.Noul > 1 {
			return nil, fmt.Errorf("noul outside [0,1]")
		}
		return a, nil
	case "choice":
		if _, err := requiredFields(raw, "choice", "confidence", "probabilities"); err != nil {
			return nil, err
		}
		var a ChoiceAnswer
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		if err := validateDistribution(a.Confidence, fields["probabilities"]); err != nil {
			return nil, err
		}
		return a, nil
	case "score":
		if _, err := requiredFields(raw, "score", "confidence", "probabilities", "legend"); err != nil {
			return nil, err
		}
		var a ScoreAnswer
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		if a.Score < 0 {
			return nil, fmt.Errorf("score is negative")
		}
		if err := validateDistribution(a.Confidence, fields["probabilities"]); err != nil {
			return nil, err
		}
		return a, nil
	default:
		return nil, fmt.Errorf("unknown answer type %q", kind)
	}
}

func validateDistribution(confidence float64, raw json.RawMessage) error {
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence outside [0,1]")
	}
	var probabilities map[string]*float64
	if err := json.Unmarshal(raw, &probabilities); err != nil {
		return err
	}
	if len(probabilities) == 0 {
		return fmt.Errorf("probabilities are empty")
	}
	for label, p := range probabilities {
		if p == nil || *p < 0 || *p > 1 {
			return fmt.Errorf("probability %q is null or outside [0,1]", label)
		}
	}
	return nil
}
