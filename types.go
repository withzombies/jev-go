package jev

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Question is one of [Noul], [Choice], [Score], or [RawQuestion]. Instructions
// and descriptions may contain text, JSON objects, or arrays. See the API for
// supported content.
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
	// Criteria lists at least one level description, starting at score zero.
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
// State must encode as a JSON string, object, or array. Empty Model uses the client default.
type Request struct {
	// ExtraBody shallow-merges over all standard fields, including explicit nulls.
	// Validation and the byte cap apply to the final encoded payload.
	ExtraBody map[string]any `json:"-"`
	// State is the shared JSON-encodable context for all questions.
	State any `json:"state"`
	// Questions maps caller-chosen identifiers to non-nil question values.
	Questions map[string]Question `json:"questions"`
	// Model selects a model name or alias; empty uses the client default.
	Model string `json:"model"`
}

// Answer is a [NoulAnswer], [ChoiceAnswer], [ScoreAnswer], or [RawAnswer],
// decoded by its wire type.
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
	// InputTokens is the reported input count; nil means absent or null.
	InputTokens *int `json:"input_tokens"`
	// OutputTokens is the reported output count; nil means absent or null.
	OutputTokens *int `json:"output_tokens"`
}

// Response contains one typed answer per question and server metadata.
type Response struct {
	// HTTPResponse holds buffered transport metadata; nil for manually decoded values.
	// It is excluded from JSON serialization and owns no open network resources.
	HTTPResponse *HTTPResponse `json:"-"`
	// Model is the resolved model used for evaluation.
	Model string `json:"model"`
	// Answers maps question identifiers to typed answer values.
	Answers map[string]Answer `json:"answers"`
	// Usage contains the token counts returned by the service.
	Usage Usage `json:"usage"`
	// RequestID is the x-typesafe-request-id response header, if present.
	RequestID string `json:"-"`
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

// HTTPResponse is a fully buffered response. The network body is already closed;
// Headers and Body belong to this result. It is not a streaming interface.
type HTTPResponse struct {
	// StatusCode is the HTTP response status.
	StatusCode int
	// Headers contains a snapshot of response headers.
	Headers http.Header
	// Body contains the original response bytes and may contain submitted data.
	Body []byte
	// RequestID is the x-typesafe-request-id header, or empty when absent.
	RequestID string
	// Endpoint is the HTTP method and relative API path.
	Endpoint string
}

// ModelsResponse holds available models and their originating HTTP response.
type ModelsResponse struct {
	// Models lists the available models and aliases.
	Models []Model `json:"models"`
	// HTTPResponse contains buffered transport metadata, excluded from JSON.
	HTTPResponse *HTTPResponse `json:"-"`
	// RequestID is the x-typesafe-request-id header, if present.
	RequestID string `json:"-"`
}

// RawAnswer preserves a future answer type without interpreting its fields.
type RawAnswer struct {
	// Type is the nonempty wire discriminator.
	Type string
	// Body is the complete JSON answer object, including its type.
	Body json.RawMessage
}

func (a RawAnswer) answerType() string { return a.Type }

// MarshalJSON preserves the original JSON answer object.
func (a RawAnswer) MarshalJSON() ([]byte, error) { return a.Body.MarshalJSON() }

// Nouls returns a new map containing only typed yes/no answers.
func (r Response) Nouls() map[string]NoulAnswer { return answerGroup[NoulAnswer](r.Answers) }

// Choices returns a new map containing only typed choice answers. Nested maps
// within each answer remain shared with Answers.
func (r Response) Choices() map[string]ChoiceAnswer { return answerGroup[ChoiceAnswer](r.Answers) }

// Scores returns a new map containing only typed score answers. Nested maps
// within each answer remain shared with Answers.
func (r Response) Scores() map[string]ScoreAnswer { return answerGroup[ScoreAnswer](r.Answers) }
func answerGroup[T Answer](answers map[string]Answer) map[string]T {
	group := make(map[string]T)
	for name, answer := range answers {
		if typed, ok := answer.(T); ok {
			group[name] = typed
		}
	}
	return group
}

// UnmarshalJSON decodes known answers strictly and preserves future answer types
// as RawAnswer. Missing required fields never become implicit zero-valued answers.
// Usage counts may be absent or null; transport metadata is not read from JSON.
func (r *Response) UnmarshalJSON(data []byte) error {
	fields, err := requiredFields(data, "model", "answers", "usage")
	if err != nil {
		return err
	}
	var result Response
	if err := decodeAt(fields["model"], &result.Model, "model"); err != nil {
		return err
	}
	if result.Model == "" {
		return at("model", fmt.Errorf("model is empty"))
	}
	if err := decodeAt(fields["usage"], &result.Usage, "usage"); err != nil {
		return err
	}
	var rawAnswers map[string]json.RawMessage
	if err := decodeAt(fields["answers"], &rawAnswers, "answers"); err != nil {
		return err
	}
	result.Answers = make(map[string]Answer, len(rawAnswers))
	for name, raw := range rawAnswers {
		answer, err := decodeAnswer(raw)
		if err != nil {
			return at("answers."+name, err)
		}
		result.Answers[name] = answer
	}
	*r = result
	return nil
}

type fieldError struct {
	path string
	err  error
}

func (e *fieldError) Error() string { return fmt.Sprintf("%s: %v", e.path, e.err) }
func (e *fieldError) Unwrap() error { return e.err }
func at(path string, err error) error {
	var nested *fieldError
	if errors.As(err, &nested) && nested.path != "" {
		if path != "" {
			path += "."
		}
		path += nested.path
	}
	return &fieldError{path: path, err: err}
}
func decodeAt(data []byte, target any, path string) error {
	if err := json.Unmarshal(data, target); err != nil {
		var typed *json.UnmarshalTypeError
		if errors.As(err, &typed) && typed.Field != "" {
			if path != "" {
				path += "."
			}
			path += typed.Field
		}
		return at(path, err)
	}
	return nil
}
func requiredFields(data []byte, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, at("", err)
	}
	for _, name := range names {
		value := bytes.TrimSpace(fields[name])
		if len(value) == 0 || bytes.Equal(value, []byte("null")) {
			return nil, at(name, fmt.Errorf("missing or null field"))
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
	if err := decodeAt(fields["type"], &kind, "type"); err != nil {
		return nil, err
	}
	if kind == "" {
		return nil, at("type", fmt.Errorf("empty answer type"))
	}
	switch kind {
	case "noul":
		if _, err := requiredFields(raw, "noul"); err != nil {
			return nil, err
		}
		var a NoulAnswer
		if err := decodeAt(raw, &a, ""); err != nil {
			return nil, err
		}
		if a.Noul < 0 || a.Noul > 1 {
			return nil, at("noul", fmt.Errorf("outside [0,1]"))
		}
		return a, nil
	case "choice":
		if _, err := requiredFields(raw, "choice", "confidence", "probabilities"); err != nil {
			return nil, err
		}
		var a ChoiceAnswer
		if err := decodeAt(raw, &a, ""); err != nil {
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
		if err := decodeAt(raw, &a, ""); err != nil {
			return nil, err
		}
		if a.Score < 0 {
			return nil, at("score", fmt.Errorf("negative score"))
		}
		if err := validateDistribution(a.Confidence, fields["probabilities"]); err != nil {
			return nil, err
		}
		return a, nil
	default:
		return RawAnswer{Type: kind, Body: append(json.RawMessage(nil), raw...)}, nil
	}
}
func validateDistribution(confidence float64, raw json.RawMessage) error {
	if confidence < 0 || confidence > 1 {
		return at("confidence", fmt.Errorf("outside [0,1]"))
	}
	var probabilities map[string]*float64
	if err := decodeAt(raw, &probabilities, "probabilities"); err != nil {
		return err
	}
	if len(probabilities) == 0 {
		return at("probabilities", fmt.Errorf("empty distribution"))
	}
	for label, p := range probabilities {
		if p == nil || *p < 0 || *p > 1 {
			return at("probabilities."+label, fmt.Errorf("null or outside [0,1]"))
		}
	}
	return nil
}
