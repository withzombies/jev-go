# GitHub documentation refresh

## Intent

Make the reusable Go module easy to discover and use through a polished README,
focused repository Markdown guides, and a generated API reference. Standard Go
doc comments and executable examples are authoritative. Keep all project
documentation on GitHub; do not add a site or external publishing workflow.

## Acceptance

- Library-first quickstart and clear navigation in the README.
- Preserve detailed library, triage, development, and SDK parity documentation.
- Pin gomarkdoc as a development-only tool; make docs regenerates docs/api.md.
- make docs-check detects stale output without editing it, locally and in CI.
- Keep handwritten Markdown checks strict and explain generated-only exceptions.
- Run all quality checks, push, verify hosted CI and GitHub rendering.

## Boundaries

No runtime behavior, public API, or triage policy changes. No runtime dependencies.
