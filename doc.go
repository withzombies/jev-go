// Package jev is a small, unofficial client for TypeSafe's System One API.
// It uses only the Go standard library and returns typed probabilistic answers.
//
// # Requests and answers
//
// Construct a Client with NewClient and call Client.SystemOne with a Request.
// A request combines JSON-encodable state with named Noul, Choice, or Score
// questions. Each question is answered independently against the same state.
// A NoulAnswer is a probability; a ChoiceAnswer selects a label; a ScoreAnswer
// contains an expected score that may be fractional. RawQuestion and Request.ExtraBody
// allow extensible payloads. Unknown answer types are retained as RawAnswer.
// Response.Nouls, Response.Choices and Response.Scores provide typed answer maps.
// Token counts are pointers: nil means unavailable, not zero. Use Client.ListModels to
// discover available models through ModelsResponse.Models. DefaultModel is a moving alias, not a pinned version.
//
// # Configuration and ownership
//
// The caller supplies credentials through Config; this package never reads the
// environment. Logging requires an injected logger. A nil Config.HTTPClient uses a ten-second
// timeout. Inject a custom *http.Client for transport and timeout control.
// The client is safe for concurrent calls, but callers must not mutate shared
// request data or the injected HTTP client while calls are in progress.
// Each method accepts a context and closes every response body. Retries are opt-in
// through Config.Retry or WithRetry; the zero policy makes one attempt. Per-call
// options override headers and attempt timeout without mutating client defaults.
// There is no pagination or input truncation. ConfigFromEnv accepts an explicit
// lookup function, and never creates a logger or enables retries.
//
// SystemOneRaw and ListModelsRaw skip typed decoding and return HTTPResponse.
// Both typed responses also retain this fully buffered metadata. Network bodies
// are already closed, and HTTP metadata is excluded from JSON serialization.
//
// # Limits and errors
//
// Config.MaxRequestBytes optionally limits the complete serialized request,
// including questions and JSON escaping. Zero disables that local limit.
// It measures bytes, not model tokens; a request passing it may still exceed
// the server's context limit. Applications decide how to select or reduce input.
//
// Use errors.As to inspect *RequestSizeError for local size rejections and
// *APIError for non-2xx responses. APIError.ErrorType may be
// "max_tokens_exceeded" for a server context rejection. APIError retains raw
// response data for diagnostics; its Body may include submitted content, so
// avoid indiscriminate logging. Wrapped transport errors preserve causes for
// errors.Is, including context cancellation. Invalid or incomplete responses
// return *ResponseValidationError with field paths and raw HTTP diagnostics.
// *TransportError preserves connection/body-read causes and exposes Timeout.
// APIError also exposes a server Message, Endpoint and optional RetryAfter delay.
//
// See https://docs.typesafe.ai/api for the service contract and model limits.
package jev
