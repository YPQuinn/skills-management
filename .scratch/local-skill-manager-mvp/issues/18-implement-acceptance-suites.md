# Implement the end-to-end acceptance suites

Type: task
Status: resolved
Blocked by: 17

## Question

Implement the release-gating acceptance suites selected by [Define MVP packaging and acceptance](09-define-mvp-acceptance.md): CLI golden and adversarial journeys, REST contracts, Chromium full WebUI journey, Firefox/WebKit smoke, accessibility and responsive checks, crash-recovery failpoints, cross-process concurrency, state-loss Store recovery, local Git fixtures, README command smoke, and the required native-platform artifact executions.

## Answer

Delivered the release-gating acceptance infrastructure and the missing state-loss recovery behavior.

### State-loss Store recovery

Added explicit `skillctl init --recover-store` and `POST /api/v1/setup {"recover_store": true}` flows. Recovery scans pinned top-level Store directory handles, adopts valid Skill directories as newly identified unbound Skills, reserves IDs around preserved internal trees, reports unrecoverable relationship state, and never rewrites Store content. Sources, Groups, Assignments, Targets, and Managed Link ownership are not guessed; existing Target links remain unmanaged. The WebUI now offers the same recovery action when setup reports `state_missing`.

Recovery closes and detaches a cached App before replacing a missing database so long-lived processes cannot continue through a deleted SQLite handle. Tests cover content preservation, invalid entries, internal Baseline/snapshot preservation, symlink replacement races, CLI/REST contracts, process restart, and unmanaged links after recovery.

### CLI, REST, crash, and concurrency acceptance

The existing golden/adversarial CLI journeys, REST route contracts, and local Git fixtures remain the primary acceptance coverage. Added process-level recovery and concurrency suites for:

- Store replacement crashes at intent, old-content-in-recovery, installed-before-commit, and committed-before-cleanup;
- Distribution create/remove crashes after filesystem mutation and before Managed Link finalization;
- simultaneous CLI and REST mutation;
- same-Target exclusive serialization and distinct-Target reconciliation;
- Store writer lock failure and later success;
- inspection-to-mutation Target replacement with external content preservation.

Crash children terminate from test-only in-process hooks; production CLI switches and environment-variable failpoints were not introduced.

### Embedded WebUI acceptance

Added Playwright suites that build and run the embedded SPA from the real `skillctl` binary rather than Vite's development server. Chromium covers setup, Sources, Import, Groups, Targets, Assignments, Distribution, synchronization, conflict safety, deep links, browser history, restart persistence, Store recovery, filesystem/REST cross-checks, absolute symlinks, and synchronized Store content. Firefox and WebKit cover startup, navigation, resource reading, and a safe mutation.

The suites also enforce critical/serious axe results including color contrast, keyboard-only creation/import/assignment/distribution/destructive confirmation with focus restoration, desktop and narrow layouts without horizontal overflow, persisted system/light/dark themes, and reduced-motion behavior. Controls are located by accessible role, label, and state.

### Release and native gates

`scripts/release.sh` now runs `check.sh`, Playwright, and the fail-closed README quickstart harness before packaging. CI asserts one candidate commit, runs the complete functional suites on Linux amd64 and macOS arm64, runs native archive smoke on all four supported OS/architecture combinations, and runs `go test -race ./...` on Linux amd64. README content remains owned by ticket 19, so ticket-18 CI does not call its harness yet; an actual release cannot pass while README commands are missing.

Issue 09 names ESLint, while the repository's implemented frontend lint contract from ticket 17 is Oxlint. This ticket preserves and reports that established deviation rather than silently claiming ESLint coverage.

Validation passed with `./scripts/check.sh`, the targeted process suites, shell syntax/diff checks, and `./scripts/e2e-webui.sh` with all eight Chromium/Firefox/WebKit tests.
