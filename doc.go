// Package jev is a small, unofficial client for TypeSafe's System One API.
// It uses only the Go standard library and returns typed probabilistic answers.
//
// # Requests and answers
//
// Construct a Client with NewClient and call Client.SystemOne with a Request.
// A request combines JSON-encodable state with named Noul, Choice, or Score
// questions. Each question is answered independently against the same state.
// A NoulAnswer is a probability; a ChoiceAnswer selects a label; a ScoreAnswer
// contains an expected score that may be fractional. Use Client.ListModels to
// discover available models. DefaultModel is a moving alias, not a pinned version.
//
// # Configuration and ownership
//
// The caller supplies credentials through Config; this package never reads the
// environment or logs request data. A nil Config.HTTPClient uses a ten-second
// timeout. Inject a custom *http.Client for transport and timeout control.
// The client is safe for concurrent calls, but callers must not mutate shared
// request data or the injected HTTP client while calls are in progress.
// Each method accepts a context, makes a single request, and closes its response
// body. There are no automatic retries, pagination, or input truncation.
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
// return errors instead of partially populated results.
//
// See https://docs.typesafe.ai/api for the service contract and model limits.
package jev
