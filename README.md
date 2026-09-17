# jev-go

A small, unofficial Go client for [TypeSafe's System One API](https://docs.typesafe.ai/api), including Jev. Standard library only, with explicit dependency injection. Requires Go 1.26 or newer.

Module: `github.com/withzombies/jev-go` · Package: `jev`

[Library usage](#use-in-another-project) · [Configuration](#configuration-and-errors) ·
[Triage](#explore-pr-questions-with-triage) · [Development](#development) ·
[Troubleshooting](#troubleshooting) · [License](#license)

Use a patched Go toolchain: Go 1.26 is the language minimum, not a recommendation
to use an old security patch. CI tests the latest Go 1.26 patch and stable Go on Linux.

The [package overview](doc.go) explains ownership, concurrency, and error handling.
Run `go doc -all .` for the local API reference. The [executable examples](example_test.go)
cover evaluation, model listing, custom HTTP configuration, and typed errors; all
run without credentials under `go test`. Once published, the same API comments and
examples appear on [pkg.go.dev](https://pkg.go.dev/github.com/withzombies/jev-go).

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
    if err != nil {
        log.Fatal(err)
    }

    result, err := client.SystemOne(context.Background(), jev.Request{
        State: map[string]any{"message": "I was charged twice."},
        Questions: map[string]jev.Question{
            "billing": jev.Noul{Instructions: "Is this about billing?"},
            "category": jev.Choice{
                Instructions: "What category describes this message?",
                Criteria:     map[string]any{"billing": nil, "technical": nil},
            },
            "urgency": jev.Score{
                Instructions: "How time-sensitive is this message?",
                Criteria:     []any{"No deadline stated", "A deadline this week", "Immediate action requested"},
            },
        },
    })
    if err != nil {
        log.Fatal(err)
    }
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
- `Config.MaxRequestBytes` is an optional cap on the complete encoded JSON request, including state, questions, model, and JSON escaping. Zero disables it; negative values are invalid. Oversized requests return `*jev.RequestSizeError` with `Size` and `Limit` before HTTP. The client never truncates input. For example, set `MaxRequestBytes: 64 * 1024` to apply a 64 KiB request budget. This is a byte cap, not a tokenizer or a guarantee that Jev will accept the request.
- `Request.Model` defaults to `jev-latest`; set a versioned model to compare repeatable experiments. The alias can change over time.
- Both methods accept a context. Transport errors retain their cause for `errors.Is` and `errors.As`.
- Non-2xx responses return `*jev.APIError` with `StatusCode`, `Body`, `Headers`, `RequestID`, and best-effort `ErrorType` from `detail.error_type`. Use `errors.As` to inspect the error; `ErrorType == "max_tokens_exceeded"` identifies a server context-limit rejection. Its error string omits the body, which may echo submitted content. Retry headers are accessible to callers; **there are no automatic retries**.
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
triage --context-bytes 16384 --json < change.patch
```

Set `TYPESAFE_AI_API_KEY` in the command's environment. It sends only the **first 24,576 stdin bytes** to TypeSafe in one evaluation request. Set `--context-bytes` to another positive byte budget. A cut through a UTF-8 character moves back to that character’s start. Remaining stdin is consumed and discarded so pipeline producers can finish, without retaining the full input. There is no pagination.

The CLI budget limits raw input, separately from the module’s optional complete-request cap; the CLI leaves that module cap disabled. Questions also consume context. Jev documents 32k tokens for state plus the longest question and 64k for state plus all questions ([model limits](https://docs.typesafe.ai/model-jaggedness/jev-1.13)). Byte limits do not measure those tokens. Server input limits and rate limits still surface as errors; context errors suggest reducing `--context-bytes`. The command does not invoke `gh`, read repository files, execute patch code, or submit reviews.

Edit `cmd/triage/questions.go` to explore the question catalog. The initial batch contains 24 narrow blocker questions, a context-sufficiency question, a change-category Choice and four informational Scores. The catalog is an experiment, not a library API. Jev answers each question independently; the command combines the results in ordinary Go code.

The provisional policy in `cmd/triage/triage.go` is:

- A blocker probability **>= 0.80** recommends `request_changes`, even when other signals are uncertain.
- Every blocker **<= 0.20**, with context sufficiency **>= 0.80**, recommends `approve`.
- Everything else recommends `needs_human_review`.

When input is truncated, these rules apply **only to the evaluated prefix**. An `approve` verdict does not approve the omitted input.

These thresholds are uncalibrated experiment defaults. A diff can omit important context; the output is a recommendation about the supplied patch, not a verified review of the entire repository. Choice and Score results are shown for exploration and do not affect the verdict. Questions explicitly treat patch content as data, but this does not guarantee resistance to misleading content.

Text output starts with the recommendation and reasons, then shows model, usage, request ID, and every named answer. `--json` returns the same information structurally, including the original answer fields and `input_bytes` (total stdin), `evaluated_bytes` (retained prefix), and `truncated`. Text reports also show the byte counts and explicitly label partial evaluations. Reports do not echo the patch. Exit status is **0 for any successful evaluation**, including request-changes and human-review recommendations; failures exit 1. This is not a CI approval gate.

`main` wires flags, environment, signals, and the client. `run` receives a one-method evaluator plus `io.Reader` and `io.Writer`; the verdict function is pure. Normal command tests use injected dependencies, and an integration test exercises the real client through a local HTTP server.

## Development

The runtime module uses only the standard library. Development tools are separate
and do not add dependencies to `go.mod`. Full checks need a patched Go 1.26 or newer,
Make, Bash, curl, and Node.js 22 or newer; CI uses Node.js 24. On a machine with an
older Go patch, select a patched toolchain per command, for example
`GOTOOLCHAIN=go1.26.8 make check`, without changing the system installation.

```sh
make tools       # Download pinned tools into ignored .bin directories.
make fmt         # Apply gofmt and goimports.
make check       # Run every check used by CI.
```

The Makefile pins golangci-lint 2.13.2, actionlint 1.7.12, govulncheck 1.8.0, and
markdownlint-cli2 0.23.2. The golangci-lint installer uses its prebuilt release and
verifies its checksum. The other Go tools install into local directories keyed by tool and Go version,
so switching toolchains cannot reuse an older analyzer;
Markdown checks use the pinned CLI through `npx`. Initial downloads and vulnerability
database updates require network access. Tool upgrades are explicit Makefile edits.

| Command | What it verifies |
| --- | --- |
| `make fmt-check` | gofmt and import formatting without rewriting files |
| `make test` | Race tests, executable examples, and coverage in `coverage.out` |
| `make build` | Compilation of the library and triage command |
| `make lint` | Linter configuration plus the explicit Go check set |
| `make workflow-lint` | GitHub Actions syntax and expressions |
| `make docs-lint` | Markdown structure, including agent instructions and task docs |
| `make vuln` | Reachable known vulnerabilities in dependencies and the Go standard library |
| `make tidy-check` | Module tidiness without editing module files |

Normal tests are offline and use local HTTP servers and injected transports,
evaluators, readers, and writers. No API key is required. Coverage is reported to
help find missing cases; it is not enforced as an arbitrary percentage target.
Go lint checks cover correctness, error handling, HTTP lifecycle, context usage,
security, API documentation, and test practices. Specific suppressions must have
an explanation; tests are not excluded wholesale.

The [CI workflow](.github/workflows/ci.yml) runs on pull requests, pushes to `main`,
and manual dispatch. Linux test jobs use Go 1.26.x and stable Go; the quality job
runs once on stable Go. Workflows use pinned actions, read-only repository access,
timeouts, and cancellation of superseded runs. They never call the live Jev API.
This checkout has not been published; hosted CI starts after it is pushed to GitHub.

[AGENTS.md](AGENTS.md) describes the repository layout, design constraints, and agent
verification workflow. Task records live in `plans/active`.

## Troubleshooting

| Symptom | Next step |
| --- | --- |
| Missing or invalid API key | Supply `Config.APIKey`; for triage, set `TYPESAFE_AI_API_KEY`. The module does not read environment variables. |
| `RequestSizeError` | Inspect `Size` and `Limit` with `errors.As`; reduce the complete request or deliberately adjust `MaxRequestBytes`. |
| `max_tokens_exceeded` | Reduce state or question content. In triage, lower `--context-bytes`. Byte budgets are not token counts. |
| HTTP 401, 429, or another service error | Inspect `APIError.StatusCode`, `Headers`, and `RequestID`; the client does not retry automatically. Treat raw `Body` as potentially sensitive. |
| Timeout or cancellation | Check the caller's context and HTTP client timeout. Wrapped errors retain their causes for `errors.Is`. |
| Truncated triage report | The verdict applies only to `evaluated_bytes`; remaining input was consumed but not evaluated. |
| Vulnerability check fails | Check the Go toolchain patch as well as module dependencies. Re-run with a patched supported toolchain; do not suppress the finding. |
| Tool download fails | Check access to GitHub, the Go module proxy, npm, and the Go vulnerability database as appropriate. |

## License

Licensed under the [Apache License, Version 2.0](LICENSE).
