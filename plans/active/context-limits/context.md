# Context and evidence

The full PR #8556 (147921 bytes) previously failed with HTTP 400 and detail.error_type=max_tokens_exceeded. A planning experiment sending only its first 24576 bytes returned all 30 answers (jev-1.13.0, 12582 input tokens). This validates one input, not a general token guarantee.

TypeSafe publishes 32k tokens for state plus longest question and 64k for state plus all questions: https://docs.typesafe.ai/model-jaggedness/jev-1.13 . No confirmed tokenizer was found; byte caps are explicitly not token measurements. Existing three-implementation research is recorded in ../jev-client/context.md (official Python SDK, official JS SDK, Vercel adapter).

User selected an opt-in library cap and a prefix-scoped verdict retaining existing policy. Pagination is deferred.

## Library TDD

RED: go test ./... failed on missing MaxRequestBytes, RequestSizeError and APIError.ErrorType. Tests exercise exact wire size (default model, questions, escaping, Unicode), zero and negative limits, rejected requests never reaching transport, and context error diagnostics including malformed bodies.

GREEN: go test -race ./..., go build ./..., go vet ./..., git diff --check passed after implementation.
