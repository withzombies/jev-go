# Context and evidence

The full PR #8556 (147921 bytes) previously failed with HTTP 400 and detail.error_type=max_tokens_exceeded. A planning experiment sending only its first 24576 bytes returned all 30 answers (jev-1.13.0, 12582 input tokens). This validates one input, not a general token guarantee.

TypeSafe publishes 32k tokens for state plus longest question and 64k for state plus all questions: <https://docs.typesafe.ai/model-jaggedness/jev-1.13> . No confirmed tokenizer was found; byte caps are explicitly not token measurements. Existing three-implementation research is recorded in ../jev-client/context.md (official Python SDK, official JS SDK, Vercel adapter).

User selected an opt-in library cap and a prefix-scoped verdict retaining existing policy. Pagination is deferred.

## Library TDD

RED: go test ./... failed on missing MaxRequestBytes, RequestSizeError and APIError.ErrorType. Tests exercise exact wire size (default model, questions, escaping, Unicode), zero and negative limits, rejected requests never reaching transport, and context error diagnostics including malformed bodies.

GREEN: go test -race ./..., go build ./..., go vet ./..., git diff --check passed after implementation.

## CLI TDD and acceptance

RED: go test ./cmd/triage failed on the missing contextBytes option. Added tests for short/exact/long/default inputs, split and complete UTF-8 characters, one evaluation after fully draining input, prefix-scoped approval and JSON/text metadata, drain failure preserving its cause, invalid flags/help, cancellation during drain, and context-limit guidance retaining the API error.

GREEN: go test -race ./..., go build ./..., go vet ./..., gofmt -l . and git diff --check passed.

Live rebuilt CLI pipeline: gh pr diff 8556 --repo Vector35/binaryninja-api --color never | ./triage --json. Exit 0, input_bytes=147921, evaluated_bytes=24576, truncated=true, 30 answers, verdict=needs_human_review, model=jev-1.13.0, usage=12582 input / 585 output tokens, request_id=req_01a0b0741a29731a99bc0347ef01b406. This is a prefix evaluation, not a full-PR review. No GitHub review was submitted.

Reviewed implementation against plan: module cap remains opt-in and counts actual encoded request bytes; API errors preserve diagnostics; CLI uses bounded prefix storage, drains remainder, sends exactly one request, preserves injection and verdict policy, and documents byte/token distinctions. No pagination or tokenizer dependency introduced.
