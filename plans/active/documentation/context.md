# Documentation context

## Research

Read the complete relevant READMEs before implementation:

- [gomarkdoc v1.1.0](https://github.com/princjef/gomarkdoc/tree/v1.1.0): Go comments and examples generate GitHub Markdown; check mode detects drift.
- [google/uuid](https://github.com/google/uuid): concise library introduction and installation.
- [go-retryablehttp](https://github.com/hashicorp/go-retryablehttp): explain transport integration with a short usage example.

Also consulted [Go doc comments](https://go.dev/doc/comment),
[executable examples](https://go.dev/blog/examples), and
[GitHub README guidance](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-readmes).
Internet search and source reading were completed during planning.

## Decisions

The user selected repository Markdown over GitHub Pages. Keep the existing SDK
parity matrix. Use gomarkdoc v1.1.0 with explicit repository/main source links;
verified generation against the current package during planning. Its native
anchors and expandable examples require narrowly scoped formatting exceptions.
No changes to the runtime module or publication elsewhere.

## Verification

Observed RED when docs/api.md was absent. After generation, docs-check passed.
A deliberate stale-output probe also failed with the expected mismatch while
leaving the file unchanged; restoring the file made the check pass. Repeated
regeneration was byte-identical. The reference also matches when generated with
Go 1.27.1 instead of Go 1.26.8.

The README quickstart compiles in a temporary directory without a live API call.
Executable examples pass offline. GOTOOLCHAIN=go1.26.8 make check passed: race
tests, build, formatting, Go linters, workflow validation, documentation freshness,
Markdown lint, vulnerability scan, and module tidiness. No runtime dependency added.

GitHub's Markdown API rendered all six reader-facing documents, preserving ten
expandable executable examples. Link checking accounts for normalized custom
anchors and derives heading slugs from rendered heading text. Hosted CI and the
published repository rendering remain to be checked after push.
