# PR triage example

[Home](../README.md) · [Library guide](guide.md) · [API reference](api.md) · [Triage](triage.md) · [Development](development.md)

An exploratory consumer of the Go module: send a patch to Jev, ask independent
questions, and combine the answers into a recommendation.

## Run a review

Run these commands from a checkout of this repository. The pipeline requires the
[GitHub CLI](https://cli.github.com/) authenticated for the target repository.
Set `TYPESAFE_API_KEY` in your environment before running triage.

Build or install the small example command locally:

```sh
go build -o triage ./cmd/triage
gh pr diff 123 --repo OWNER/REPO | ./triage

# Install into GOBIN (or GOPATH/bin) for the short pipeline, with that directory on PATH:
go install ./cmd/triage
gh pr diff 123 --repo OWNER/REPO | triage

# Saved patches and local changes work too:
git diff | triage --json
triage --model jev-1.13.0 < change.patch
triage --context-bytes 16384 --json < change.patch
```

Replace `OWNER/REPO` and `123` with the target repository and pull request.

## Input and context limits

The command sends only the **first 24,576 stdin bytes** to TypeSafe in one evaluation request. Set `--context-bytes` to another positive byte budget. A cut through a UTF-8 character moves back to that character’s start. Remaining stdin is consumed and discarded so pipeline producers can finish, without retaining the full input. There is no pagination.

The CLI budget limits raw input, separately from the module’s optional complete-request cap; the CLI leaves that module cap disabled. Questions also consume context. Jev documents 32k tokens for state plus the longest question and 64k for state plus all questions ([model limits](https://docs.typesafe.ai/model-jaggedness/jev-1.13)). Byte limits do not measure those tokens. Server input limits and rate limits still surface as errors; context errors suggest reducing `--context-bytes`. The command does not invoke `gh`, read repository files, execute patch code, or submit reviews.

## Questions and verdicts

Edit [the question catalog](../cmd/triage/questions.go) to explore the question catalog. The initial batch contains 24 narrow blocker questions, a context-sufficiency question, a change-category Choice and four informational Scores. The catalog is an experiment, not a library API. Jev answers each question independently; the command combines the results in ordinary Go code.

The provisional policy in [the evaluator](../cmd/triage/triage.go) is:

- A blocker probability **>= 0.80** recommends `request_changes`, even when other signals are uncertain.
- Every blocker **<= 0.20**, with context sufficiency **>= 0.80**, recommends `approve`.
- Everything else recommends `needs_human_review`.

> [!IMPORTANT]
> When input is truncated, these rules apply **only to the evaluated prefix**.
> An `approve` verdict does not approve the omitted input.

These thresholds are uncalibrated experiment defaults. A diff can omit important context; the output is a recommendation about the supplied patch, not a verified review of the entire repository. Choice and Score results are shown for exploration and do not affect the verdict. Questions explicitly treat patch content as data, but this does not guarantee resistance to misleading content.

## Reports and exit status

Text output starts with the recommendation and reasons, then shows model, usage, request ID, and every named answer. `--json` returns the same information structurally, including the original answer fields and `input_bytes` (total stdin), `evaluated_bytes` (retained prefix), and `truncated`. Text reports also show the byte counts and explicitly label partial evaluations. Reports do not echo the patch. Exit status is **0 for any successful evaluation**, including request-changes and human-review recommendations; failures exit 1. This is not a CI approval gate.

## Dependency injection

`main` wires flags, environment, signals, and the client. `run` receives a one-method evaluator plus `io.Reader` and `io.Writer`; the verdict function is pure. Normal command tests use injected dependencies, and an integration test exercises the real client through a local HTTP server.
