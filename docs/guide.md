# Library guide

[Home](../README.md) · [Library guide](guide.md) · [API reference](api.md) · [Triage](triage.md) · [Development](development.md)

Start with the [quickstart](../README.md#quickstart). This guide covers the choices
applications make when configuring and calling the client. The
[API reference](api.md) documents every exported declaration and includes tested examples.

## Questions and answers

`Noul` returns a probability of yes without a separate confidence field. `Choice` returns a label, probabilities and confidence. `Score` returns a possibly fractional expected score, probabilities, confidence and a legend. Score map keys remain strings, as on the wire. Instructions and criteria can contain structured JSON; use `NoulCriteria` to optionally describe true/false outcomes.

Build a question set with caller-chosen identifiers:

```go
request := jev.Request{
    State: "I was charged twice.",
    Questions: map[string]jev.Question{
        "billing": jev.Noul{Instructions: "Is this about billing?"},
        "category": jev.Choice{
            Instructions: "What category describes this message?",
            Criteria:     map[string]any{"billing": nil, "technical": nil},
        },
        "urgency": jev.Score{
            Instructions: "How time-sensitive is this message?",
            Criteria: []any{
                "No deadline stated",
                "A deadline this week",
                "Immediate action requested",
            },
        },
    },
}
```

Pass `request` to `client.SystemOne(ctx, request)`. The returned answer maps use
the same identifiers: `billing`, `category`, and `urgency`.

`Response` includes the resolved model, nullable input/output token counts, server
request ID, and a buffered `HTTPResponse` with status, headers, and original body.
`client.ListModels(ctx)` returns a `*ModelsResponse`; iterate its `Models` field.
Its request ID and HTTP metadata are available too. HTTP metadata is excluded from
JSON serialization. Network bodies are already closed before either method returns.

`result.Nouls()`, `result.Choices()`, and `result.Scores()` return typed answer maps.
Use the map's normal comma-ok lookup to distinguish missing answers from zero values.
The returned maps are new; nested maps within answers remain shared. Token counts
are `*int`: nil means absent or null, and a pointer to zero means a reported zero.

For extensible payloads, use `RawQuestion` and `Request.ExtraBody`. Raw questions
preserve unknown fields and explicit JSON nulls. ExtraBody shallowly overrides
standard fields, including state, model, and questions; objects are replaced.
Validation and the byte cap apply to this final payload. Known score questions need
at least one criterion, matching the live API. Unknown answer types are retained as
`RawAnswer`; known types remain strictly validated against the effective questions.

`SystemOneRaw` and `ListModelsRaw` return buffered `*HTTPResponse` values without
typed decoding. They use the same request options, retries, byte cap, and non-2xx
error handling. These methods do not stream and require no response-body cleanup.
See the [capability matrix](../SDK_PARITY.md) for the pinned upstream versions and
explicit mappings of Python/TypeScript features to Go.

## Client configuration

Pass explicit configuration to `jev.NewClient`. Construction validates settings
without making an HTTP request. Reuse a client across calls.

| Setting | Default | Purpose |
| --- | --- | --- |
| `APIKey` | Required | Bearer credential supplied by the application |
| `BaseURL` | `https://api.typesafe.ai` | API endpoint; override for a proxy or local test server |
| `DefaultModel` | `jev-latest` | Model used when `Request.Model` is empty |
| `HTTPClient` | New client, 10-second timeout | Inject transport and timeout behavior |
| `Timeout` | Inherit the HTTP client timeout | Override the timeout for each attempt |
| `Headers` | None | Additional request headers, copied at construction |
| `Retry` | Disabled | Explicit retry policy; zero value makes one attempt |
| `MaxRequestBytes` | Disabled (`0`) | Cap the complete encoded request in bytes |
| `Logger` | None | Inject a `*slog.Logger` to enable diagnostics |
| `LogLevel` | `warn` | Filter diagnostics when a logger is supplied |
| `LogBodies` | `false` | Include body contents only when debug logging is also enabled |

`NewClient` does not read environment variables. Applications choose their
credential source. `ConfigFromEnv(os.Getenv)` is an opt-in helper that reads
`TYPESAFE_API_KEY`, `TYPESAFE_BASE_URL`, `TYPESAFE_DEFAULT_MODEL`, and
`TYPESAFE_LOG_LEVEL` through the supplied lookup function. Blank values are
ignored. Override fields on the returned config before passing it to `NewClient`;
the helper does not create a logger or enable retries. Tests can inject a lookup
function instead of using the process environment.

`Request.Model` overrides the client default. The `jev-latest` alias can change;
use an available versioned model for repeatable experiments. Discover models
with `client.ListModels(ctx)`.

## Ownership and dependency injection

The supplied `*http.Client` remains caller-owned: the library never modifies or
closes it. Inject an `http.RoundTripper` through that client for custom transport
behavior. Clients support concurrent calls; callers must not mutate shared
request data or HTTP client configuration while calls are in progress.

Consumers can define interfaces containing only the methods they need. The
module does not impose an application interface or DI framework. Tests in this
repository use local HTTP servers and injected transports; see the
[executable examples](../example_test.go) and [development guide](development.md).

## Per-call options and deadlines

All endpoint methods accept `context.Context` first. Use a context deadline to
bound the entire call, including retry waits. `WithTimeout` controls each attempt,
including reading its response body; it leaves the supplied HTTP client unchanged.

Use `WithHeaders`, `WithTimeout`, and `WithRetry` for per-call overrides. Later
options take precedence. Header maps and retry status slices are snapshotted.
Authentication, JSON protocol, SDK identity, and retry count headers remain
protected. A retry override replaces the entire policy;
`WithRetry(jev.RetryPolicy{})` disables retries for that call.

## Retries

Retries are **disabled by default**. Opt in with
`Retry: jev.DefaultRetryPolicy()` in client configuration or
`jev.WithRetry(jev.DefaultRetryPolicy())` on one call.

| Default policy setting | Value |
| --- | --- |
| Retries after the initial attempt | 2 |
| Exponential backoff | Starts at 500ms, capped at 5s, with 25% jitter |
| HTTP statuses | 408, 429, and 500–599 |
| Transport failures | Connection errors and attempt timeouts |
| Server delay hints | Honor hints up to 60s |

The policy prefers `retry-after-ms` over `Retry-After` (seconds or HTTP date).
Invalid hints or hints over the configured ceiling fall back to backoff. A zero
`MaxRetryAfter` removes that ceiling. Edit the returned policy to change these
settings. Caller cancellation always stops retries. Replaying an evaluation can
result in additional billable service work.

## Request and context limits

`MaxRequestBytes` caps the final serialized JSON, including state, questions,
model, extra fields, and escaping. Zero disables this cap; negative values are
invalid. For example, `MaxRequestBytes: 64 * 1024` sets a 64 KiB request budget.
Oversized requests return `*jev.RequestSizeError` before HTTP; inspect `Size` and
`Limit` with `errors.As`.

> [!NOTE]
> A byte budget is not a token count. Passing the local check does not guarantee
> the request fits the model's context window.

The library does not truncate or paginate input. Applications decide how to
select or reduce state and questions. A service context rejection appears as
`*jev.APIError` with `ErrorType == "max_tokens_exceeded"`. The
[triage example](triage.md#input-and-context-limits) applies its own stdin budget.

## Errors and diagnostics

Use `errors.As` for typed diagnostics and `errors.Is` for wrapped causes,
including context cancellation and deadlines.

| Error | Meaning | Useful fields or methods |
| --- | --- | --- |
| `*RequestSizeError` | Local encoded request exceeds its byte budget | `Size`, `Limit` |
| `*APIError` | Service returned a non-2xx status | `StatusCode`, `ErrorType`, `RequestID`, `Headers`, `Endpoint`, `RetryAfter` |
| `*TransportError` | Connection or response-body read failed | `Unwrap`, `Timeout`, `Endpoint` |
| `*ResponseValidationError` | Response is malformed, incomplete, or mismatches requested answer types | `FieldPath`, `HTTPResponse`, `Unwrap` |

Further request constraints are validated by the service. Use standard HTTP
status constants with `APIError.StatusCode` instead of language-specific
exception subclasses. `RetryAfter` is nullable. `APIError.Message` exposes a
best-effort server message; `Body` retains the original response. Error strings
omit both because they can contain submitted content. Treat raw response bodies,
server messages, and validation diagnostics as potentially sensitive data.

Logging requires an injected `*slog.Logger`. `LogLevel` accepts `debug`, `info`,
`warn`, `error`, and `off`; the logger's handler also filters messages. Credential
headers are redacted. Bodies remain omitted unless `LogBodies` is true and debug
logging is enabled.

## Troubleshooting

| Symptom | Next step |
| --- | --- |
| Missing or invalid API key | Supply `Config.APIKey`; for triage, set `TYPESAFE_API_KEY`. The module does not read environment variables. |
| `RequestSizeError` | Inspect `Size` and `Limit` with `errors.As`; reduce the complete request or deliberately adjust `MaxRequestBytes`. |
| `max_tokens_exceeded` | Reduce state or question content. In triage, lower `--context-bytes`. Byte budgets are not token counts. |
| HTTP 401, 429, or another service error | Inspect `APIError.StatusCode`, `Headers`, and `RequestID`; retries require an explicit policy. Treat raw `Body` as potentially sensitive. |
| Timeout or cancellation | Check the caller's context and HTTP client timeout. Wrapped errors retain their causes for `errors.Is`. |
| Truncated triage report | The verdict applies only to `evaluated_bytes`; remaining input was consumed but not evaluated. |
