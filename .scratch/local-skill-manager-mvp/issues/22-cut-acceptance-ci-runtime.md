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

Verification is local only so far. Runs [34100751825](https://github.com/YPQuinn/skills-management/actions/runs/34100751825) on `main` and [34102203547](https://github.com/YPQuinn/skills-management/actions/runs/34102203547) on this branch both fail in `record candidate` before any step executes, with `The job was not started because recent account payments have failed or your spending limit needs to be increased`. The `main` failure predates this branch, so it is an account billing block rather than a workflow regression. This ticket stays claimed until a real run confirms the new step timings.

The billing block also reframes the cost side. The repository is private, so Actions minutes are billed and the two macOS jobs carry a 10x multiplier: `darwin/arm64 functional` and `darwin/amd64 archive smoke` together account for most of the per-push spend even though `linux/amd64 functional` dominates wall clock. Of the 115s `darwin/amd64 archive smoke` job, 55s is `setup-go` alone.

## Follow-up: how often the matrix runs, and on what

Re-reading the workflow for spend rather than wall clock exposed a structural problem the step timings hid: one commit was triggering the matrix two or three times.

`on: push` carried no branch filter alongside `on: pull_request`, so every push to a PR branch ran the full matrix twice. Four separate commits show the pair — for example `d9ac56f2` produced a `pull_request` run and a `push` run three seconds apart. The unfiltered `push` also matched tags, so `v0.1.1` ran `acceptance` on `main`, then `acceptance` again on the tag, then `release archives` — three matrices for one commit, and `release.sh` inside the third already runs `check.sh`.

`push` is now limited to `main`. Tags belong to `release-archives.yml`. A `concurrency` group cancels superseded pull-request runs.

The `record candidate` job is removed along with the four `same candidate` steps. It checked out a runner to assert `git rev-parse HEAD` equals `GITHUB_SHA`, which is true by construction, and every job then re-asserted it against that output. Ticket 09's requirement is that all platforms exercise one candidate commit, and GitHub already guarantees this: every job in a run checks out `GITHUB_SHA`. The job also made all four platforms wait on it through `needs`, adding roughly 35s of wall clock per run for a guarantee the platform provides. Ticket 18's claim that "CI asserts one candidate commit" now holds through the platform rather than through a dedicated job.

`darwin/amd64 archive smoke` is removed from the per-push matrix. It built a throwaway `skillctl_ci_darwin_amd64.tar.gz`, not a release artifact, and cost about 20 billable-minute equivalents per push at the 10x macOS rate. Ticket 09 asks the two non-functional platforms to run the *release* smoke, which `release-archives.yml` already does against the real archives and `SHA256SUMS` on four native runners. A `CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build` step on the linux job keeps early warning of build breakage for about six seconds.

`darwin/arm64 functional` no longer runs the full `check.sh`. TypeScript, Oxlint, and Vitest are platform-independent and already run on Linux; Vitest alone was 36s of the macOS job. The macOS-specific risk is in the Go suite, which the code comments tie to `/var` to `/private/var` resolution and APFS behavior. The job now runs `build.sh`, `go test ./...`, and the native archive smoke. The frontend build cannot be dropped entirely because `internal/webui` declares `//go:embed all:dist` and `internal/webui/dist` is gitignored, so no package importing it compiles until Vite has run — the fail-closed property ticket 09 asked for. Skipping the frontend build on macOS is also defensible in the other direction: `release-archives.yml` packages on `ubuntu-latest`, so the SPA that actually ships is always a Linux build.
