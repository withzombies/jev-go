# jev-go

A small, unofficial Go client for [TypeSafe's System One API](https://docs.typesafe.ai/api), including Jev. Standard library only, with explicit dependency injection. Requires Go 1.26 or newer.

Module: `github.com/withzombies/jev-go` · Package: `jev`

## Use in another project

Once the repository is published:

```sh
go get github.com/withzombies/jev-go
```

For local development before publishing, add a `replace` directive in the consuming project's `go.mod` pointing to this checkout:

```go
require github.com/withzombies/jev-go v0.0.0

replace github.com/withzombies/jev-go => /absolute/path/to/jev-triage
```

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    jev "github.com/withzombies/jev-go"
)

func main() {
    client, err := jev.NewClient(jev.Config{
        APIKey: os.Getenv("TYPESAFE_AI_API_KEY"),
    })
    if err != nil { log.Fatal(err) }

    result, err := client.SystemOne(context.Background(), jev.Request{
        State: map[string]any{"message": "I was charged twice."},
        Questions: map[string]jev.Question{
            "billing": jev.Noul{Instructions: "Is this about billing?"},
            "category": jev.Choice{
                Instructions: "What category describes this message?",
                Criteria: map[string]any{"billing": nil, "technical": nil},
            },
            "urgency": jev.Score{
                Instructions: "How time-sensitive is this message?",
                Criteria: []any{"No deadline stated", "A deadline this week", "Immediate action requested"},
            },
        },
    })
    if err != nil { log.Fatal(err) }
    fmt.Println(result.Answers["billing"].(jev.NoulAnswer).Noul)
    fmt.Println(result.Answers["category"].(jev.ChoiceAnswer).Choice)
    fmt.Println(result.Answers["urgency"].(jev.ScoreAnswer).Score)
}
```

`Noul` returns a probability of yes without a separate confidence field. `Choice` returns a label, probabilities and confidence. `Score` returns a possibly fractional expected score, probabilities, confidence and a legend. Score map keys remain strings, as on the wire. Instructions and criteria can contain structured JSON; use `NoulCriteria` to optionally describe true/false outcomes.

`Response` also includes the resolved model, input/output token usage, and server request ID. `client.ListModels(ctx)` returns available model names, descriptions and release dates.

## Configuration and errors

- `Config.APIKey` is required. The **library does not read environment variables**; applications choose where credentials come from. The Python SDK uses `TYPESAFE_API_KEY`; the example application here uses `TYPESAFE_AI_API_KEY`.
- `Config.BaseURL` defaults to `https://api.typesafe.ai`. Set it for an HTTP proxy or local test server.
- `Config.HTTPClient` defaults to a client with a 10-second timeout. Supply your own `*http.Client` and `http.RoundTripper` for transport behavior. The supplied client is never mutated or closed.
- `Request.Model` defaults to `jev-latest`; set a versioned model to compare repeatable experiments. The alias can change over time.
- Both methods accept a context. Transport errors retain their cause for `errors.Is` and `errors.As`.
- Non-2xx responses return `*jev.APIError` with `StatusCode`, `Body`, `Headers`, and `RequestID`. Its error string omits the body, which may echo submitted content. Retry headers are accessible to callers; **there are no automatic retries**.
- Malformed responses, missing answers and mismatched answer types return errors. Further request constraints are validated by the service.
- Reuse clients concurrently, but do not mutate shared requests or HTTP client configuration during calls.

Consumers can define interfaces containing only the methods they need. The module does not impose an application interface or DI framework.

## Explore PR questions with `triage`

Build or install the small example command locally:

```sh
go build -o triage ./cmd/triage
gh pr diff 8556 | ./triage

# Install into GOBIN (or GOPATH/bin) for the short pipeline, with that directory on PATH:
go install ./cmd/triage
gh pr diff 8556 | triage

# Saved patches and local changes work too:
git diff | triage --json
triage --model jev-1.13.0 < change.patch
```

Set `TYPESAFE_AI_API_KEY` in the command's environment. It sends the **complete stdin content to TypeSafe** in a single evaluation request, with no truncation or chunking. Server input limits and rate limits surface as errors. It does not invoke `gh`, read repository files, execute patch code, or submit reviews.

Edit `cmd/triage/questions.go` to explore the question catalog. The initial batch contains 24 narrow blocker questions, a context-sufficiency question, a change-category Choice and four informational Scores. The catalog is an experiment, not a library API. Jev answers each question independently; the command combines the results in ordinary Go code.

The provisional policy in `cmd/triage/triage.go` is:

- A blocker probability **>= 0.80** recommends `request_changes`, even when other signals are uncertain.
- Every blocker **<= 0.20**, with context sufficiency **>= 0.80**, recommends `approve`.
- Everything else recommends `needs_human_review`.

These thresholds are uncalibrated experiment defaults. A diff can omit important context; the output is a recommendation about the supplied patch, not a verified review of the entire repository. Choice and Score results are shown for exploration and do not affect the verdict. Questions explicitly treat patch content as data, but this does not guarantee resistance to misleading content.

Text output starts with the recommendation and reasons, then shows model, usage, request ID, and every named answer. `--json` returns the same information structurally, including the original answer fields. Reports do not echo the patch. Exit status is **0 for any successful evaluation**, including request-changes and human-review recommendations; failures exit 1. This is not a CI approval gate.

`main` wires flags, environment, signals, and the client. `run` receives a one-method evaluator plus `io.Reader` and `io.Writer`; the verdict function is pure. Normal command tests use injected dependencies, and an integration test exercises the real client through a local HTTP server.

## Development

```sh
go test -race ./...
go build ./...
go vet ./...
```

Normal tests are offline and use `httptest.Server`, injected HTTP transports, and complete response fixtures. The external-package executable example also runs without credentials. Work follows observed RED → GREEN → REFACTOR increments; evidence is recorded in `plans/active/jev-client/context.md`.
