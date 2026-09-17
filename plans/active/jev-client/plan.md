# Jev Go client

Build module `github.com/withzombies/jev-go`, package `jev`, using Go 1.26 and the standard library. The library is the deliverable; `gh pr diff 8556 | triage` is an exploratory consumer under `cmd/triage`.

Expose NewClient(Config), SystemOne(context.Context, Request), and ListModels(context.Context). Inject *http.Client and base URL; accept an explicit key. Support typed Noul, Choice, Score requests/answers, structured JSON descriptions, metadata, errors and cancellation. No automatic retries or review policy in the library.

The example injects a one-method evaluator and io.Reader/io.Writer; main wires flags, environment and signals. Send one editable question batch and print answers with a provisional verdict. Blocker >= .8 requests changes; all blockers <= .2 and sufficient context >= .8 approves; otherwise human review. Support --model and --json. Never invoke gh or submit reviews.

Use RED/GREEN/REFACTOR for each behavioral increment, with observed failures recorded. Test real HTTP encoding/decoding through httptest; use test-only stubs at external boundaries. Before commits: go build ./..., go test -race ./..., go vet ./..., clean gofmt. Commit only green increments.

Acceptance: independently importable client with working examples, offline deterministic tests, and a small stdin command using the public API. Document usage, assumptions and limitations. A live synthetic smoke check supplements automated tests.
