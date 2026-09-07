# Cut acceptance CI runtime

Type: task
Status: claimed

## Question

Every push waits about ten minutes on `acceptance.yml`. Which of that time is real coverage and which is duplicated work, and what can be removed without weakening the release gate defined by [Define MVP packaging and acceptance](09-define-mvp-acceptance.md)?

## Evidence

Step timings from the fourteen most recent completed runs of the workflow, plus log timestamps from cold run 34097707082 and warm run 34090406008.

The five jobs run in parallel, so wall clock is set by the slowest one. `linux/amd64 functional` averages 601s against 257s for `darwin/arm64 functional`; the other three jobs finish in under two minutes and wait. Three steps account for 94% of the linux job: `quality gate` 216s, `race` 212s, `Playwright` 136s.

`setup-go` caches the Go build and test cache across runs, so the linux job has two regimes. A commit that touches Go code pays 242s for `quality gate` and 286s for `race`; a commit that does not pays 130s and 31s.

Three sources of duplicated work:

- `check.sh` runs `go test ./...` and the next step runs `go test -race ./...` over the same packages. The race build cache is separate, so the first run buys nothing. Cold cost of the redundant run: 134s.
- The `Playwright` step's `scripts/e2e-webui.sh` calls `build.sh`, which does `npm ci` and `rm -rf dist && vite build` again after `check.sh` already produced a dist in the same job. Cost: 21s.
- `npx playwright install --with-deps` reinstalls apt system libraries (41s) and redownloads about 510 MB of browsers (22s) on every run. Actual test execution is 54s of the 136s step.

`internal/app` dominates the Go suite at 127.6s plain and 273.1s under `-race`, more than half of each. The cause is not one slow test: 230 top-level tests sum to 135s with the slowest at 2.7s. None of them call `t.Parallel()`, so the package runs strictly serially and the machine idles — a local baseline run measured 14% CPU. Each test builds an isolated App over `t.TempDir()` with its own SQLite database through `newTestApp`; only `targets_test.go` shares process state, via `t.Setenv("HOME", …)`.

`internal/skillstore` looks similar but is not parallel-safe. `testBeforeDeleteUnlinkHook`, `testDrainChildHook`, `testAfterMkdirHook`, and `testAfterOpDirEntrySync` are package-level seams that 18 of its 196 tests assign. Parallelizing the package makes a hook set by one test fire inside another; a measurement run failed with `the operation-directory sync seam must fire exactly once, fired 10` and a cross-test `Fatal` from `TestStageCleanupRefusesUnknownChild` raised during `TestRestoreReplaceMissingEverythingIsEmptyState`.

## Acceptance

- Keep `go test -race ./...` running on Linux amd64, as ticket 09 requires.
- Keep the embedded Chromium/Firefox/WebKit acceptance from [Implement the end-to-end acceptance suites](18-implement-acceptance-suites.md) on the release path, not on every push.
- Do not weaken the four-platform native archive smoke.
- Parallelize only tests proven isolated; leave packages with shared process state serial.

## Answer

Per-push CI now covers the CLI and the Go/frontend quality gate; the WebUI browser journeys move to the release path only.

`scripts/check.sh` takes an optional `GO_TEST_FLAGS`. The linux job sets it to `-race` and the separate `race` step is gone, so the suite runs once instead of twice. Other callers, including `release.sh` and local development, are unchanged.

Both functional jobs drop the `Playwright` step and its failure-artifact upload, and their `setup-node` cache no longer references `e2e/webui/package-lock.json`. `scripts/e2e-webui.sh` and the Playwright suites are untouched, and `release.sh` still runs them between `check.sh` and packaging, so an actual release cannot skip WebUI acceptance.

225 of the 230 `internal/app` tests now call `t.Parallel()`. The five in `targets_test.go` stay serial because `setTestHome` uses `t.Setenv`. Measured on the same machine, the package goes from 134.1s to 18.9s and CPU utilization from 14% to 81%; under `-race` it finishes in 32.2s with no race reported.

`internal/skillstore` was deliberately left serial. Making it parallel needs the four package-level hooks reworked into per-test state first, which is a separate change.
