# SDK feature parity

[Home](README.md) · [Library guide](docs/guide.md) · [API reference](docs/api.md)

The reusable Go module covers the applicable public capabilities of both official
SDKs at these audited versions:

- [Python 0.6.0, commit 420ef4f](https://github.com/typesafe-ai/typesafe-sdk-python/tree/420ef4ffb612d5a539a1e0f0fe883ff6770340af).
- [TypeScript 0.6.0, commit 66880cc](https://github.com/typesafe-ai/typesafe-sdk-js/tree/66880ccded6cb642dc1809620c2b108c33730214).
- [Live HTTP contract](https://api.typesafe.ai/openapi.json), inspected September 17, 2026.

This is capability parity at pinned versions, not a promise to copy every language
construct or automatically track future releases. Both upstream SDKs expose only
System One evaluation and model listing. Neither provides a context tokenizer,
streaming evaluation, or automatic pagination.

## Capability matrix

| SDK capability | Go implementation or native equivalent | Verification |
| --- | --- | --- |
| Evaluation and model discovery | `SystemOne`, `ListModels`, `ModelsResponse` | `TestSystemOneWireRequest`, `TestListModels`, `TestModelListMetadata` |
| Noul, Choice, Score builders and discriminated answers | Struct literals and typed `Question` / `Answer` interfaces | `TestQuestionsMarshalWireTypes`, `TestResponseDecodesTypedAnswersAndRoundTrips` |
| Structured state, instructions and rubric descriptions | JSON-encodable values, including nested objects and arrays | Wire tests and `TestEffectiveRequestAndRawQuestions` |
| Optional instructions, explicit nulls and extra question fields | Typed optional fields; `RawQuestion` preserves explicit nulls and arbitrary fields | `TestSingleScoreCriterionAndExplicitNull`, effective-request tests |
| Extra top-level request fields and shallow overrides | `Request.ExtraBody`; validation and byte cap use the effective payload | `TestEffectiveRequestAndRawQuestions`, `TestExtraBodyByteLimit`, `TestQuestionValidationUsesEffectivePayload` |
| Client model/base URL/key defaults | Explicit `Config`; per-request model overrides the client default | `TestClientConfigValidation`, `TestClientDefaultsAndCallHeaders` |
| Environment configuration | Opt-in `ConfigFromEnv` with an injected lookup and all four SDK variable names | `TestConfigFromEnv`, `ExampleConfigFromEnv` |
| Custom HTTP clients/transports | `Config.HTTPClient` and standard `http.RoundTripper` | `TestInjectedHTTPClientAndBodyClosure`, `TestTransportErrorIsWrapped` |
| Client and call-specific headers | `Config.Headers`, `WithHeaders`; snapshots and protected SDK/authentication headers | `TestClientDefaultsAndCallHeaders`, `TestRetryReplayAndPolicyIsolation` |
| Client and call-specific timeouts | `Config.Timeout`, `WithTimeout`, existing HTTP client timeout | `TestPerCallTimeoutDoesNotMutateHTTPClient`, `TestAttemptTimeoutRetry` |
| Retry count, status selection, backoff, jitter, transport switches | Opt-in `RetryPolicy`, `DefaultRetryPolicy`, `WithRetry` | Retry selection, clock, replay, isolation and validation tests |
| Retry hints and delay ceilings | Milliseconds, seconds and HTTP dates; configurable hint ceiling | `TestRetryDelay`, `TestRetryClockAndCancellation`, API-error diagnostics |
| Python custom exception/predicate retries | `AdditionalRetry`, with `errors.Is` / `errors.As`, including response validation failures | `TestRetrySelectionAndCallOverride`, `TestAdditionalRetryCanHandleResponseValidation` |
| Python total retry budget / JS cancellation | Caller context deadline/cancellation spans attempts and waits | `TestRetryClockAndCancellation`, cancellation tests |
| Typed result plus raw HTTP response | `Response.HTTPResponse`, `ModelsResponse.HTTPResponse`, request IDs | Metadata and effective-request tests |
| Raw response without typed parsing | `SystemOneRaw`, `ListModelsRaw`; bodies buffered and closed before return | `TestRawEndpointsSkipDecoding`, body-closure tests |
| Grouped answers | `Nouls()`, `Choices()`, `Scores()` | `TestAnswerGroupsAndOptionalUsage` |
| Optional token counts and fractional scores | Nullable `*int` counts and `float64` scores | Optional-usage and typed round-trip tests; CLI unknown-usage test |
| Future response fields and answer types | Unknown fields retained in raw body; future types become `RawAnswer` | Effective-request round-trip and model metadata tests |
| HTTP error subclasses | `errors.As` into `*APIError`, then standard HTTP status constants | `TestAPIErrorsPreserveMetadataWithoutRetrying`, `TestAPIErrorDiagnostics` |
| Server messages, endpoint, request ID and retry hints | Explicit `APIError` fields; body excluded from `Error()` | API-error diagnostics and metadata tests |
| Connection, timeout and cancellation errors | `TransportError`, `Timeout()`, wrapped causes, standard context errors | Transport, body-read, timeout and cancellation tests |
| Python response validation errors and field paths | `ResponseValidationError` retains field path, cause and HTTP diagnostics | `TestResponseValidationDiagnostics`, malformed-response tests |
| Logging levels, injected logger, redaction and wire diagnostics | Opt-in `slog.Logger`; `LogLevel` and separate `LogBodies` opt-in | `TestOptInLogging` |
| SDK/runtime/retry identification headers | Protected `jev-go`, Go runtime/platform and retry-count headers | Header and replay tests |
| Serialization, copying and package version inspection | `encoding/json`, normal Go values, `go list -m` / `runtime/debug.ReadBuildInfo` | JSON round-trip tests, examples, build and module checks |
| Async clients, promises and result transforms | One concurrency-safe client with goroutines, contexts and ordinary Go functions | `TestClientConcurrentReuse`, cancellation and race tests |
| Python context-manager cleanup | Network bodies closed by the SDK; supplied HTTP clients stay caller-owned | `TestInjectedHTTPClientAndBodyClosure`, retry replay tests |
| TypeScript inferred types / Python typed dictionaries | Concrete Go answer types, typed group accessors and `RawQuestion` | Compile-checked external-package tests and executable examples |
| Browser guards, ESM/CommonJS and Python packaging internals | Language/runtime-specific; this module targets Go applications and uses Go modules | Go 1.26/stable CI build and module checks |

## Deliberate Go policies

- The zero retry policy makes exactly one SDK attempt. `DefaultRetryPolicy()` must
  be explicitly selected. Per-call policies replace the whole value; edit a copy
  to override selected fields.
- Context deadlines are hard total bounds, unlike Python's retry-scheduling budget.
  A context deadline also interrupts an attempt already in progress. Attempt
  timeouts can be retried; caller cancellation and deadlines cannot.
- `NewClient` never reads the environment or creates a logger. `ConfigFromEnv`
  returns values for the caller to override and does not choose a log sink.
- A supplied HTTP client is never mutated or closed. Callers who need connection
  pool cleanup can supply their own client and call its `CloseIdleConnections`.
- Raw responses are buffered, not streaming. Both raw methods retain normal
  non-2xx error handling. Transport metadata is excluded from typed JSON output.
- Unknown answers are preserved rather than discarded. Missing requested answers
  and mismatched discriminators remain errors. Known answers are decoded strictly.
- Score maps retain JSON string keys. The live API and Python allow a one-entry
  rubric; the TypeScript SDK currently requires two. Go follows the live minimum.
- Nullable usage counts accommodate Python-supported responses; the live schema
  currently marks both counts required. Missing counts remain distinguishable
  from reported zero counts.
- Error strings omit server bodies. Server messages are explicitly accessible;
  debug body logging requires opt-in because content can contain application data.
- Request byte budgets are an additional Go capability. They measure final JSON
  bytes, not model tokens. Truncation and input selection remain application policy.

## Maintaining parity

For an upstream update, compare public exports, configuration, endpoint methods,
wire types and behavior tests at a new pinned commit. Update this matrix, add
failing regression tests for new applicable capabilities, and run the full quality
gates. Record upstream differences explicitly instead of weakening existing tests.
