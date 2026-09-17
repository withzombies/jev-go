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
