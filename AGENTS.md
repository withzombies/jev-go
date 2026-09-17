# Agent instructions

## Project shape

This repository publishes `github.com/withzombies/jev-go`, package `jev`: a thin,
standard-library Go client for TypeSafe's System One API. The reusable module is
the main deliverable. `cmd/triage` is an exploratory stdin application, not a
library dependency or a GitHub automation service.

- Root Go files implement the HTTP client, wire types, and errors.
- Root external-package tests and executable examples exercise the public API.
- `cmd/triage` owns flags, environment, process I/O, the question catalog, and verdict policy.
- `plans/active` holds task intent, decisions, and verification evidence.
- The Makefile and lint configuration define the checks used locally and in CI.

## Go design

Follow [Effective Go](https://go.dev/doc/effective_go),
[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), and
[Go Doc Comments](https://go.dev/doc/comment).

- Prefer simple, explicit code and the standard library. Keep development tools
  out of the published module's dependency graph.
- Accept narrow interfaces where the consumer needs them; return concrete types.
  Preserve explicit HTTP, evaluator, and I/O injection. Do not add a DI framework.
- Pass `context.Context` first and propagate it to blocking work. Do not store it
  in client structs or replace a caller's context with a background context.
- Document ownership and concurrency guarantees. Never mutate or close a
  caller-owned HTTP client. Always release response bodies and other resources.
- Check errors, add useful operation context, and preserve causes with `%w`.
  Use `errors.Is` and `errors.As`. Explain intentional best-effort discards;
  never discard an error merely to satisfy a linter.
- Keep configuration explicit. The library must not read environment variables,
  access process I/O, exit the process, log secrets, or impose triage policy.
- Preserve the distinction between byte budgets and token context limits.
  The library rejects oversized requests; truncation belongs to the application.
- Avoid unnecessary abstractions, global mutable state, implicit retries,
  speculative features, and unmeasured optimizations.
- Remove dead code. When an approved change replaces an API, remove the old API
  rather than adding compatibility shims or deprecation layers.

## Work and testing

1. Inspect the relevant implementation and tests. Before coding, study three
   comparable implementations using at least three credible internet sources;
   read the relevant implementations fully and record applicable decisions.
2. For behavior changes, write a focused failing test and observe the expected
   failure before implementing the smallest change that passes.
3. Refactor only while tests remain green. Use meaningful existing checks for
   documentation, formatting, and configuration changes rather than artificial tests.
4. Test real behavior through the public API, local HTTP servers, and injected
   dependencies. Cover relevant malformed responses, boundaries, errors,
   cancellation, resource cleanup, and concurrency.
5. Keep normal tests and examples offline and credential-free. Live Jev calls
   belong to explicitly authorized manual experiments, never CI.
6. Never disable a failing test. Avoid broad lint exclusions; a necessary
   suppression must name the linter and explain the specific reason.
7. Make incremental commits that compile and pass all checks. Explain why the
   change is needed in the commit message. Record actual verification results,
   including limitations; do not claim a hosted CI run from local checks alone.

## Documentation

Document every exported declaration and the meaning of exported fields, including
units, defaults, zero values, error behavior, ownership, and concurrency where
relevant. Keep the package overview, README, executable examples, and CLI help
consistent with the code. Examples should compile and run under `go test`.
Update documentation in the same change as the behavior it describes.

Use Apache-2.0 for this repository and preserve the canonical LICENSE text.
Do not add contributor policy documents unless explicitly requested.

## Verification commands

Use a patched Go 1.26 or newer toolchain. CI tests the latest 1.26 patch and stable
Go on Linux. Full quality checks additionally need Make, Bash, curl, and Node.js
22 or newer (CI uses Node.js 24). Tool versions are pinned in the Makefile.

```sh
make tools          # Install pinned Go tools into ignored .bin directories.
make fmt            # Apply gofmt and goimports; this edits files.
make fmt-check      # Check formatting without rewriting files.
make test           # Race tests, executable examples, and coverage report.
make build          # Compile all packages.
make lint           # Validate configuration and run all selected Go linters.
make workflow-lint  # Validate GitHub Actions workflows.
make docs-lint      # Validate Markdown; downloads the pinned CLI via npx.
make vuln           # Scan reachable vulnerabilities using the Go database.
make tidy-check     # Check module tidiness without editing module files.
make check          # All verification targets above except tools and fmt.
```

Tool installation and vulnerability database access require network access; the
Go test suite itself does not. Coverage is diagnostic, not an arbitrary percentage
gate. Before each commit, `make check` must pass with zero lint findings and
`git diff --check` must be clean. Do not suppress a vulnerability report to pass
CI; check the toolchain version as well as package dependencies.
