# SDK parity

Implement the union of Python and TypeScript SDK capabilities using explicit,
idiomatic Go APIs. The reusable module is the deliverable; triage is a consumer.
Keep the runtime standard-library-only. Retries, environment loading, and logging
are opt-in. No compatibility shims are required.

## Accepted changes

- Client defaults and per-call headers, timeout, and complete retry-policy overrides.
- Injected environment lookup and logger; protected headers and credential redaction.
- Configurable retries, jitter, retry hints, cancellation, and deterministic tests.
- Raw questions, shallow extra-body overrides, effective-payload validation and byte caps.
- Buffered raw responses, model-list metadata, typed answer groups, unknown answers,
  nullable token counts, transport errors, and response-validation field paths.
- Documentation and a complete capability matrix, with executable offline examples.

## Acceptance

Use RED-GREEN-REFACTOR for each behavior slice. Test real public API behavior with
local HTTP servers and injected dependencies. Preserve context propagation,
resource ownership, request replay, and concurrency. All Makefile checks must pass
before each commit. Push and verify all hosted CI jobs, fixing failures until green.

## Defaults and boundaries

Use the live API's minimum of one score criterion. Raw request extras override
standard fields shallowly. Validate the effective request. Preserve unknown answer
objects while rejecting missing requested answers and type mismatches. Total time
budgets use caller contexts. Keep string score keys. CLI changes only adapt the
library API and display unavailable token counts as unknown.
