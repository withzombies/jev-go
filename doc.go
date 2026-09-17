// Package jev is a small, unofficial client for TypeSafe's System One API.
// It uses only the Go standard library and returns typed probabilistic answers.
//
// # Requests and answers
//
// Construct a [Client] with [NewClient] and call [Client.SystemOne] with a
// [Request]. A request combines JSON-encodable state with named [Noul], [Choice],
// or [Score] questions. Each question is answered independently against the same
// state. A [NoulAnswer] is a probability; a [ChoiceAnswer] selects a label;
// a [ScoreAnswer] contains an expected score that may be fractional.
//
// [RawQuestion] and Request.ExtraBody allow extensible payloads. Unknown answer
// types are retained as [RawAnswer]. [Response.Nouls], [Response.Choices], and
// [Response.Scores] provide typed answer maps. Token counts in [Usage] are
// pointers: nil means unavailable, not zero. Use [Client.ListModels] to discover
// available models through ModelsResponse.Models. [DefaultModel] is a moving
// alias, not a pinned version.
//
// # Configuration and ownership
//
// The caller supplies credentials through [Config]; this package never reads
// the environment. [ConfigFromEnv] accepts an explicit lookup function and never
// creates a logger or enables retries. Logging requires an injected logger.
//
// A nil Config.HTTPClient uses a ten-second timeout. Inject a custom *http.Client
// for transport and timeout control. The client is safe for concurrent calls,
// but callers must not mutate shared request data or the injected HTTP client
// while calls are in progress. Each method accepts a context and closes every
// response body. There is no pagination or input truncation.
//
// Retries are opt-in through Config.Retry or [WithRetry]; the zero policy makes
// one attempt. [DefaultRetryPolicy] returns a configurable retry policy.
// [WithHeaders] and [WithTimeout] override headers and attempt timeout without
// mutating client defaults. A context deadline bounds the entire call, including
// retry waits. Per-attempt timeouts include reading the response body.
//
// [Client.SystemOneRaw] and [Client.ListModelsRaw] skip typed decoding and return
// [HTTPResponse]. Both typed responses also retain this fully buffered metadata.
// Network bodies are already closed, and HTTP metadata is excluded from typed
// response JSON serialization.
//
// # Limits and errors
//
// Config.MaxRequestBytes optionally limits the complete serialized request,
// including questions and JSON escaping. Zero disables that local limit.
// It measures bytes, not model tokens; a request passing it may still exceed
// the server's context limit. Applications decide how to select or reduce input.
//
// Use errors.As to inspect [RequestSizeError] for local size rejections and
// [APIError] for non-2xx responses. APIError.ErrorType may be
// "max_tokens_exceeded" for a server context rejection. APIError retains raw
// response data for diagnostics; its Body and Message may include submitted
// content. Its error string omits both. Endpoint and optional RetryAfter provide
// additional diagnostics without enabling retries.
//
// [TransportError] preserves connection/body-read causes for errors.Is and
// errors.As, including context cancellation, and exposes a Timeout method.
// Invalid or incomplete responses return [ResponseValidationError] with field
// paths and raw HTTP diagnostics.
//
// See the [TypeSafe API documentation] for the service contract and model limits.
//
// [TypeSafe API documentation]: https://docs.typesafe.ai/api
package jev
