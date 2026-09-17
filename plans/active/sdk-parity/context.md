# SDK parity context

## Sources inspected before implementation

- [Python SDK 0.6.0](https://github.com/typesafe-ai/typesafe-sdk-python/tree/420ef4ffb612d5a539a1e0f0fe883ff6770340af): read public exports, both clients, request preparation, transport, types, configuration, retries, errors, response decoding, and logging; inspected relevant upstream test cases.
- [TypeScript SDK 0.6.0](https://github.com/typesafe-ai/typesafe-sdk-js/tree/66880ccded6cb642dc1809620c2b108c33730214): read client, types, questions, retries, errors, environment, logging, API promise, and models implementations; inspected reliability regressions.
- [Go HTTP retry implementation](https://github.com/hashicorp/go-retryablehttp/blob/fd004584a46724fae09e2f21d7c382e15c893f42/client.go): read complete client implementation; use replayable bytes, cancellable waits, and explicit HTTP ownership, without adding a runtime dependency.
- [Live API](https://api.typesafe.ai/openapi.json): only System One and models endpoints; score minItems is one. Python permits absent token counts; TypeScript score validation is stricter than the live schema.

## Grounding

Root client.go, types.go, errors.go and their external-package tests are the main
implementation seams. cmd/triage owns process I/O, environment and verdict policy.
Existing race tests passed with Go 1.26.8 before changes. Working tree was clean.

## Decisions

The user chose explicit Go defaults. SDK capabilities map to native Go constructs
where appropriate: goroutines, contexts, status codes, errors.Is/As and slog.
Preserve caller-owned HTTP clients. No live API calls are needed for parity tests.

## Evidence

Implementation verification will be recorded here as each slice completes.

Configuration and retry/logging slices observed RED on absent public APIs, then
GREEN on the complete test suite. Retry timing uses testing/synctest; delay
calculation accepts explicit time and randomness, without mutable globals.

First slice: Go 1.26.8 make check passed (race tests, build, formatting, all 26
linters, workflow and Markdown checks, vulnerability scan, module tidiness).
Library coverage was 95.1%. Explicit test-only lint annotations explain intentionally
noncanonical header keys and dummy credentials used to verify redaction.

Second slice: observed RED for missing raw payload/response APIs; added them and
adapted callers without shims. Existing tests caught typed-nil question marshaling
and pointer token rendering regressions. Individual question marshaling preserves
nil safety; CLI prints unknown for unavailable counts (observed failing test first).
An additional failing test established that custom retry predicates must also see
response-validation errors; decoding now occurs within the shared attempt loop.
Full Go 1.26.8 make check passed before the final documentation/examples additions.

Final local review matched the capability matrix to implementation and tests.
A boundary test exposed float rounding above an int64-duration cap; the retry
calculation now preserves the integer cap before converting back. The test was
observed failing before the fix. New examples are executed by go test.

Both GOTOOLCHAIN=go1.26.8 make check and GOTOOLCHAIN=go1.27.1 make check passed,
including race tests, executable examples, build, formatting, zero linter findings,
workflow and Markdown validation, no reachable vulnerabilities and module tidiness.
The module still has no runtime dependencies. Hosted CI verification is recorded below.

## Completion evidence

Implementation commit 522fc7f passed every hosted job in
[CI run 35265782275](https://github.com/withzombies/jev-go/actions/runs/35265782275):
Go 1.26.x (10s), Go stable (44s), and Quality (28s). No hosted fixes were needed.
The ignored local triage executable was rebuilt and its --help smoke test passed.
The capability matrix in SDK_PARITY.md records all applicable audited capabilities
and intentional Go equivalents. No live service calls or credentials were used for
parity verification. No runtime dependencies or compatibility shims were added.
