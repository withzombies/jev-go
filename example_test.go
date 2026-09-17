package jev_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	jev "github.com/withzombies/jev-go"
)

func ExampleClient_SystemOne() {
	// A local server keeps this executable example independent of credentials.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"model":"jev-test","usage":{"input_tokens":10,"output_tokens":2},"answers":{"billing":{"type":"noul","noul":0.98}}}`)
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
