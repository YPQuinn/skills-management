# Implement Distribution and adoption

Type: task
Status: resolved
Blocked by: 13, 14

## Question

Implement the complete Distribution vertical slice: fresh Target safety checks, desired and observed status, Managed Link ownership, dry-run planning, no-overwrite creation and verified removal, adoption, per-item and Target outcomes, durable link intents and recovery, multi-process lock ordering, and equivalent Target status and action flows across CLI, REST, and Appica WebUI.

## Answer

Delivered the complete Distribution vertical slice across the Go core, SQLite, CLI, REST, and bilingual Appica WebUI.

- Added durable Managed Link, Distribution Status, per-item/Target outcome, and link-intent state with version-9 migration compatibility.
- Added fresh coherent inspection, stale-observation retention, desired/observed planning, dry-run, explicit adoption, and no automatic overwrite or adoption.
- Made Target filesystem operations relative to pinned, component-wise `O_NOFOLLOW` directory handles. Link creation uses atomic no-overwrite `symlinkat`; removal uses a persisted private isolation directory, immediate verification, safe restoration, and crash recovery without trusting recorded absolute intent paths.
- Added Store-shared then Target-exclusive cross-process locking for inspection, reconciliation, adoption, and recovery.
- Added equivalent noun-first CLI commands, strict REST endpoints, and responsive Appica Target status/action flows with English and Simplified Chinese copy.
- Added migration, state-machine, adversarial filesystem, crash-window, lock/concurrency, CLI/REST, WebUI, and end-to-end tests.

Validation passed: `go test ./...`, touched-package race tests, `go vet ./...`, Darwin/Linux builds, WebUI TypeScript/Vitest/Oxlint/production build, `git diff --check`, and final deep review.
