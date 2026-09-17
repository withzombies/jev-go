package jev

import (
	"encoding/json"
	"fmt"
)

// RawQuestion forwards a JSON object, including extra fields and future types.
// A nonempty string "type" is required. Explicit nil values encode as JSON null.
// Known choice and score criteria receive the same structural checks as typed questions.
type RawQuestion map[string]any

func (q RawQuestion) questionType() string { kind, _ := q["type"].(string); return kind }

// MarshalJSON encodes the complete question without dropping unknown fields.
func (q RawQuestion) MarshalJSON() ([]byte, error) { return json.Marshal(map[string]any(q)) }

// MarshalJSON shallow-merges ExtraBody over the standard request fields. Objects
// are replaced, never deep-merged. The caller's maps are not modified.
func (r Request) MarshalJSON() ([]byte, error) {
	fields := map[string]any{"state": r.State, "questions": r.Questions, "model": r.Model}
	for key, value := range r.ExtraBody {
		fields[key] = value
	}
	// Encode question interfaces individually so typed nil pointers become JSON
	// null instead of invoking a value-receiver marshaler through an interface.
	if questions, ok := fields["questions"].(map[string]Question); ok && questions != nil {
		encoded := make(map[string]json.RawMessage, len(questions))
		for name, question := range questions {
			raw, err := json.Marshal(question)
			if err != nil {
				return nil, fmt.Errorf("question %q: %w", name, err)
			}
			encoded[name] = raw
		}
		fields["questions"] = encoded
	}
	return json.Marshal(fields)
}

func (c *Client) prepareSystemOne(request Request) ([]byte, map[string]string, error) {
	if request.Model == "" {
		request.Model = c.defaultModel
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, nil, fmt.Errorf("jev: encode request: %w", err)
	}
	var payload struct {
		Questions map[string]json.RawMessage `json:"questions"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, fmt.Errorf("jev: decode effective questions: %w", err)
	}
	if len(payload.Questions) == 0 {
		return nil, nil, fmt.Errorf("jev: at least one question is required")
	}
	kinds := make(map[string]string, len(payload.Questions))
	for name, raw := range payload.Questions {
		fields, err := requiredFields(raw, "type")
		if err != nil {
			return nil, nil, fmt.Errorf("jev: question %q: %w", name, err)
		}
		var kind string
		if err := json.Unmarshal(fields["type"], &kind); err != nil || kind == "" {
			return nil, nil, fmt.Errorf("jev: question %q needs a nonempty string type", name)
		}
		switch kind {
		case "score":
			var criteria []json.RawMessage
			if err := json.Unmarshal(fields["criteria"], &criteria); err != nil || len(criteria) == 0 {
				return nil, nil, fmt.Errorf("jev: score question %q needs at least one criterion in an array", name)
			}
		case "choice":
			var criteria map[string]json.RawMessage
			if err := json.Unmarshal(fields["criteria"], &criteria); err != nil || criteria == nil {
				return nil, nil, fmt.Errorf("jev: choice question %q needs a criteria object", name)
			}
		}
		kinds[name] = kind
	}
	if c.maxRequestBytes > 0 && len(body) > c.maxRequestBytes {
		return nil, nil, &RequestSizeError{Size: len(body), Limit: c.maxRequestBytes}
	}
	return body, kinds, nil
}
