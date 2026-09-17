# Context

Initial workspace empty, no Git repository. Go 1.26.4 and gh available. TYPESAFE_AI_API_KEY present (never record its value). Module path chosen by user. User explicitly requires TDD and DI and wants the CLI only as an experiment.

## Research (2026-09-17)

- Official Python SDK: https://github.com/typesafe-ai/typesafe-sdk-python/tree/420ef4ffb612d5a539a1e0f0fe883ff6770340af/src/typesafe_sdk/_core — studied synchronous client, questions, response decoding, errors, config and transport.
- Official JS SDK: https://github.com/typesafe-ai/typesafe-sdk-js/tree/66880ccded6cb642dc1809620c2b108c33730214/src — studied client, questions, types, model listing.
- Third implementation: https://github.com/vercel/ai/blob/main/packages/gateway/src/gateway-evaluation-model.ts — read evaluation adapter; its gateway protocol differs from the direct API.
- Direct API: https://docs.typesafe.ai/api and https://api.typesafe.ai/openapi.json
- Context/limitations: https://docs.typesafe.ai/model-jaggedness/jev-1.13.md

Live GET /v1/models and synthetic POST /v1/systemone succeeded during planning. Model resolved to jev-1.13.0. Noul has only a probability; Score can be fractional. Usage has input_tokens and output_tokens, not billing_units. Current OpenAPI permits one score criterion, while prose suggests at least two; avoid duplicating server limits.

## Design

Inject a standard *http.Client, leaving ownership with caller. Consumer-defined interfaces belong in the command, not a library-wide abstraction. Question/answer variants use small JSON discrimination code. No global mutable configuration. Only main accesses process I/O and credentials. All normal tests remain offline.

## TDD evidence

- Types RED: `go test ./...` failed because no production Go package existed. GREEN after implementing question encoding and discriminated answer decoding.
- HTTP RED: client tests failed on missing Client/Config/NewClient/APIError. GREEN: `go test -race ./...` passed with real local HTTP tests, injected transport fault tests and concurrent client reuse.
- Malformed numeric response RED: seven cases accepted invalid probabilities/confidences/scores and one accepted an empty model. GREEN after narrowly adding numeric distribution and model checks.
- `go build ./...`, `go vet ./...`, and `git diff --check` passed for the library increment.
- CLI RED: command tests failed on missing run/assess/catalog/command types and functions. GREEN: `go test -race ./...` passed including exact threshold boundaries, invalid evidence, dependency errors, empty input, option parsing, and an actual-client/httptest integration.
- Offline coverage: library 93.2%, command 93.3%. Build and vet passed after adding the command.
- Go directive normalized to 1.26.0 to match the documented Go 1.26 minimum, rather than unnecessarily requiring the installed patch release.
- Cancellation regression RED: `TestCommandCancellationUnblocksInput` reproduced waiting on an open input pipe after context cancellation. GREEN fix: the composition root accepts an explicit io.ReadCloser and closes process input on cancellation using context.AfterFunc. The injected core run function still only requires io.Reader.

## Final verification and review

- A separate temporary Go module imported this checkout with a local replace directive and used the actual Go client against TypeSafe: listed two models and received typed Noul, Choice and Score answers from jev-1.13.0 (362 input / 62 output tokens).
- Actual CLI pipeline with a synthetic arithmetic patch: all 30 answers returned, JSON decoded successfully, verdict approve, model jev-1.13.0 (3285 input / 585 output tokens). This verifies integration, not review accuracy. No real PR content was sent.
- Post-cancellation-fix gates: go test -race ./..., go build ./..., go build -o triage ./cmd/triage, go vet ./..., gofmt -l . and git diff --check all passed. The local ignored ./triage binary is ready to run.
- Reviewed against plan: two endpoint methods, standard-library dependencies, concrete public client, explicit HTTP injection, typed JSON questions/answers, client ownership and cancellation, no retries/global configuration, independent consumer smoke, editable stdin example, pure policy and complete reports. No TODO/FIXME or subprocess/GitHub calls in production code.
- Library is locally usable and committed; no remote repository was created or published. README documents both local replace usage and future installation.
