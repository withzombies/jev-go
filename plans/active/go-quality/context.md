# Research and evidence

## Sources inspected before implementation

- [google/go-github CI](https://github.com/google/go-github/blob/master/.github/workflows/tests.yml) and its linter workflow: minimum/stable matrix, race tests, pinned actions, read-only permissions.
- [go-retryablehttp CI](https://github.com/hashicorp/go-retryablehttp/blob/main/.github/workflows/pr-unit-tests.yaml), actionlint workflow, and linter configuration: shared local commands, coverage, workflow validation.
- [slack-go CI](https://github.com/slack-go/slack/blob/master/.github/workflows/test.yml) and linter configuration: Go 1.26/1.27 matrix, explicit linter list, module tidiness checks.
- [Official Go documentation guidance](https://go.dev/doc/comment): defaults, exported fields, ownership, concurrency, executable examples.
- [golangci-lint installation](https://golangci-lint.run/docs/welcome/install/local/): use prebuilt distribution to avoid tool dependency interference.
- [Apache-2.0 text](https://www.apache.org/licenses/LICENSE-2.0.txt): canonical license.

## Decisions

User selected full setup, a broad explicit linter set, Linux-only CI, no contributor
rules, and Apache-2.0. Keep Go 1.26 language minimum but verify on patched toolchains.
Tool dependencies stay outside root go.mod. Makefile pins tool versions and supplies
CI commands; Markdown uses pinned npx without creating a Node project.

## Baseline

Tests passed. Proposed linter selection found 33 issues (20 errcheck, 12 revive,
one gosec dummy-credential fixture). govulncheck found five reachable standard
library vulnerabilities on installed Go 1.26.4; the same scan on Go 1.26.8 passed.
No Git remote exists. Workflow execution cannot be verified on GitHub yet.

## Implementation and verification

- Observed RED: the pinned prebuilt golangci-lint with repository configuration
  reproduced all 33 baseline findings. GREEN: zero findings after handling test
  response writes, explicitly documenting best-effort cleanup/diagnostics, naming
  unused parameters, adding public documentation, and narrowly suppressing the
  intentionally invalid credential fixture. No library behavior was changed.
- Markdown validation found 12 formatting issues in historical task records;
  these were fixed without excluding the records. README Go code was formatted
  and rendered with spaces to satisfy Markdown indentation checks.
- Four new executable examples cover custom HTTP configuration, model listing,
  local request-size errors, and remote API error metadata. All remain offline.
- The README's complete Go example compiled in a separate temporary consuming
  module using a local replace directive. Linux cross-compilation also passed.
- Full checks passed on Go 1.26.8. A Go 1.27.1 run then exposed reuse of a scanner
  built with Go 1.26: the parser rejected newer standard-library syntax. Building
  the same scanner with Go 1.27 confirmed the cause. Source-built tool paths now
  include the selected Go version, preventing reuse across toolchain switches.
- Updated the planned govulncheck pin from v1.1.4 to v1.8.0 after checking the
  actual Go module release; GitHub's latest-release metadata was stale. Both
  Go 1.26.8 and 1.27.1 scans with v1.8.0 report no vulnerabilities.
- Full `GOTOOLCHAIN=go1.27.1 make check` passed: formatting, race tests, build,
  lint configuration, zero Go lint findings, actionlint, Markdown, vulnerability
  scan, and module tidiness. Go 1.27 coverage was 93.8% library / 94.1% command.
- Reviewed against the plan: all selected linters enabled, no blanket test
  exclusions, no runtime dependencies added, canonical Apache-2.0 license,
  agent-focused instructions, public documentation, matching local/CI commands,
  pinned read-only Linux workflows, and no live API calls in CI.
- Checks ran locally on macOS, including a Linux cross-build. Actual Linux test
  execution on GitHub remains unverified because this repository has no remote.
