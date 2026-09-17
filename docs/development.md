# Development

[Home](../README.md) · [Library guide](guide.md) · [API reference](api.md) · [Triage](triage.md) · [Development](development.md)

## Requirements

The runtime module uses only the standard library. Development tools are separate
and do not add dependencies to `go.mod`. Full checks need a patched Go 1.26 or newer,
Make, Bash, curl, and Node.js 22 or newer; CI uses Node.js 24. On a machine with an
older Go patch, select a patched toolchain per command, for example
`GOTOOLCHAIN=go1.26.8 make check`, without changing the system installation.

## Local checks

```sh
make tools       # Download pinned tools into ignored .bin directories.
make fmt         # Apply gofmt and goimports.
make check       # Run every check used by CI.
```

The Makefile pins golangci-lint 2.13.2, actionlint 1.7.12, govulncheck 1.8.0, and
markdownlint-cli2 0.23.2, and gomarkdoc 1.1.0. The golangci-lint installer uses its prebuilt release and
verifies its checksum. The other Go tools install into local directories keyed by tool and Go version,
so switching toolchains cannot reuse an older analyzer;
Markdown checks use the pinned CLI through `npx`. Initial downloads and vulnerability
database updates require network access. Tool upgrades are explicit Makefile edits.

| Command | What it verifies |
| --- | --- |
| `make fmt-check` | gofmt and import formatting without rewriting files |
| `make test` | Race tests, executable examples, and coverage in `coverage.out` |
| `make build` | Compilation of the library and triage command |
| `make lint` | Linter configuration plus the explicit Go check set |
| `make workflow-lint` | GitHub Actions syntax and expressions |
| `make docs-check` | Generated API documentation matches Go comments and examples |
| `make docs-lint` | Markdown structure, including agent instructions and task docs |
| `make vuln` | Reachable known vulnerabilities in dependencies and the Go standard library |
| `make tidy-check` | Module tidiness without editing module files |

## Testing and dependency injection

Normal tests are offline and use local HTTP servers and injected transports,
evaluators, readers, and writers. No API key is required. Coverage is reported to
help find missing cases; it is not enforced as an arbitrary percentage target.
Go lint checks cover correctness, error handling, HTTP lifecycle, context usage,
security, API documentation, and test practices. Specific suppressions must have
an explanation; tests are not excluded wholesale.

## GitHub Actions

The [CI workflow](../.github/workflows/ci.yml) runs on pull requests, pushes to `main`,
and manual dispatch. Linux test jobs use Go 1.26.x and stable Go; the quality job
runs once on stable Go. Workflows use pinned actions, read-only repository access,
timeouts, and cancellation of superseded runs. They never call the live Jev API.
Hosted CI verifies pushes to the published GitHub repository.

[AGENTS.md](../AGENTS.md) describes the repository layout, design constraints, and agent
verification workflow. Task records live in [plans/active](../plans/active).

## Maintaining documentation

The documentation lives in this repository. Standard
[Go doc comments](https://go.dev/doc/comment) and
[executable examples](https://go.dev/blog/examples) are the API source of truth.

1. Update exported declaration comments and the package overview in
   [doc.go](../doc.go). Use `[Symbol]` links for Go types and functions.
2. Keep examples in [example_test.go](../example_test.go) offline and executable
   under `go test`. Document defaults, units, errors, ownership, and concurrency.
3. Update the relevant guide and [SDK parity matrix](../SDK_PARITY.md) when behavior changes.
4. Run `make docs`, then `make check`. Commit the source and generated reference together.

```sh
make docs        # Regenerate docs/api.md from the root library package.
make docs-check  # Compare without rewriting files; stale documentation fails CI.
go doc -all .    # Read the same API comments locally.
```

[gomarkdoc](https://github.com/princjef/gomarkdoc) is pinned in the Makefile;
[its configuration](../.gomarkdoc.yml) fixes GitHub source links to `main` for
reproducible output. It is a development tool, not a module dependency. Do not
hand-edit [the generated API reference](api.md). Generated-only lint settings
accommodate Go tabs, HTML anchors, expandable examples, and the generator's
formatting; handwritten Markdown keeps the repository's normal lint rules.
CI checks documentation but does not deploy a documentation site.

## Working on a consumer locally

Add a `replace` directive in the consuming project's `go.mod` pointing to this checkout:

```go
require github.com/withzombies/jev-go v0.0.0

replace github.com/withzombies/jev-go => /absolute/path/to/jev-triage
```

## Troubleshooting checks

| Symptom | Next step |
| --- | --- |
| API reference is stale | Edit the Go comments or examples, then run `make docs` and commit the generated file. |
| Vulnerability check fails | Check the Go toolchain patch as well as module dependencies. Re-run with a patched supported toolchain; do not suppress the finding. |
| Tool download fails | Check access to GitHub, the Go module proxy, npm, and the Go vulnerability database as appropriate. |
