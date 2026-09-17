package jev_test

import (
	"encoding/json"
	"reflect"
	"testing"

	jev "github.com/withzombies/jev-go"
)

func TestQuestionsMarshalWireTypes(t *testing.T) {
	tests := []struct {
		name     string
		question jev.Question
		want     string
	}{
		{"noul", jev.Noul{Instructions: "Urgent?", Criteria: &jev.NoulCriteria{True: "Now", False: "Later"}}, `{"type":"noul","instructions":"Urgent?","criteria":{"true":"Now","false":"Later"}}`},
		{"choice", jev.Choice{Instructions: map[string]any{"task": "Classify"}, Criteria: map[string]any{"bug": nil, "feature": []string{"new behavior"}}}, `{"type":"choice","instructions":{"task":"Classify"},"criteria":{"bug":null,"feature":["new behavior"]}}`},
		{"score", jev.Score{Criteria: []any{"low", map[string]any{"description": "high"}}}, `{"type":"score","criteria":["low",{"description":"high"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.question)
			if err != nil {
				t.Fatal(err)
			}
			var a, b any
			if err := json.Unmarshal(got, &a); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.want), &b); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

const mixedResponse = `{"model":"jev-1.13.0","usage":{"input_tokens":12,"output_tokens":8},"answers":{
 "urgent":{"type":"noul","noul":0},
 "category":{"type":"choice","choice":"bug","confidence":0.8,"probabilities":{"bug":0.9,"feature":0.1}},
 "severity":{"type":"score","score":0.25,"confidence":0.6,"probabilities":{"0":0.75,"1":0.25},"legend":{"0":"low","1":{"description":"high"}}}
}}`

func TestResponseDecodesTypedAnswersAndRoundTrips(t *testing.T) {
	var got jev.Response
	if err := json.Unmarshal([]byte(mixedResponse), &got); err != nil {
		t.Fatal(err)
	}
	if got.Model != "jev-1.13.0" || got.Usage.InputTokens != 12 || got.Usage.OutputTokens != 8 {
		t.Fatalf("metadata: %+v", got)
	}
	if v, ok := got.Answers["urgent"].(jev.NoulAnswer); !ok || v.Noul != 0 {
		t.Fatalf("noul: %#v", got.Answers["urgent"])
	}
	if v, ok := got.Answers["category"].(jev.ChoiceAnswer); !ok || v.Choice != "bug" || v.Probabilities["bug"] != .9 {
		t.Fatalf("choice: %#v", v)
	}
	if v, ok := got.Answers["severity"].(jev.ScoreAnswer); !ok || v.Score != .25 || v.Legend["1"] == nil {
		t.Fatalf("score: %#v", v)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var again jev.Response
	if err := json.Unmarshal(encoded, &again); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, again) {
		t.Fatalf("round trip changed response: %s", encoded)
	}
}

func TestResponseRejectsMalformedAnswers(t *testing.T) {
	for _, answer := range []string{
		`{"type":"noul"}`, `{"type":"noul","noul":null}`, `{"type":"noul","noul":"0"}`,
		`{"type":"noul","noul":1.1}`, `{"type":"noul","noul":-0.1}`,
		`{"type":"other"}`, `null`, `{"noul":0}`,
		`{"type":"choice","choice":"bug"}`,
		`{"type":"choice","choice":"bug","confidence":1.1,"probabilities":{"bug":1}}`,
		`{"type":"choice","choice":"bug","confidence":1,"probabilities":{"bug":null}}`,
		`{"type":"choice","choice":"bug","confidence":1,"probabilities":{"bug":-0.1}}`,
		`{"type":"choice","choice":"bug","confidence":1,"probabilities":{}}`,
		`{"type":"score","score":0,"confidence":-0.1,"legend":{"0":"low"},"probabilities":{"0":1}}`,
		`{"type":"score","score":-1,"confidence":1,"legend":{"0":"low"},"probabilities":{"0":1}}`,
		`{"type":"score","score":0,"confidence":1,"legend":{"0":"low"},"probabilities":{"0":1.1}}`,
		`{"type":"score","score":0,"confidence":1,"legend":{},"probabilities":null}`,
	} {
		t.Run(answer, func(t *testing.T) {
			var response jev.Response
			err := json.Unmarshal([]byte(`{"model":"jev","usage":{"input_tokens":1,"output_tokens":1},"answers":{"x":`+answer+`}}`), &response)
			if err == nil {
				t.Fatal("expected malformed answer error")
			}
		})
	}
}

func TestResponseRejectsMissingEnvelope(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"model":"","answers":{},"usage":{"input_tokens":0,"output_tokens":0}}`, `{"model":"jev","answers":null,"usage":{}}`, `{"model":"jev","answers":{},"usage":null}`} {
		var response jev.Response
		if err := json.Unmarshal([]byte(body), &response); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
