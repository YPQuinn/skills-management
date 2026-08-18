# Implement the build and release pipeline

Type: task
Status: resolved
Blocked by: 16

## Question

Implement the accepted packaging contract: locked frontend installation, generated Appica build output, `build.sh`, `check.sh`, and `release.sh`, version/commit injection, clean-tag validation, four CGO-free archives, SHA256SUMS, native artifact smoke entry points, and proof that the extracted binary runs its embedded UI and migrations without Node or repository files.

## Answer

Delivered the three accepted top-level entry points: `scripts/build.sh` performs a locked fresh Appica build and writes the native development binary to `dist/skillctl`; `scripts/check.sh` runs the ordered TypeScript, Oxlint, Vitest, Vite, formatting, vet, and Go test gates; and `scripts/release.sh` requires a clean checkout at an exact `vMAJOR.MINOR.PATCH` tag before rebuilding `dist/` as exactly four `CGO_ENABLED=0` archives plus `SHA256SUMS` with injected version and commit metadata.

Added `scripts/smoke/archive.sh` as the portable native-runner entry point. It extracts a single-binary archive into an isolated temporary environment, runs the binary with an empty `PATH` and no repository files, initializes embedded SQLite migrations, queries a migrated business table, starts the embedded Appica UI, verifies the API, SPA shell, hashed asset and deep link, then requires a clean SIGTERM shutdown. Linux runners additionally reject dynamically linked artifacts. The release command smokes only its native artifact and explicitly reports the other three as requiring native runners; full cross-platform and functional acceptance remains with the next ticket.

Validation passed for `scripts/check.sh`, `scripts/build.sh`, shell syntax, the native development archive smoke, strict release argument and dirty/tag rejection, four-platform cross-compilation, checksum/archive manifests, missing-WebUI fail-closed behavior, and a complete clean tagged release in a disposable Git repository. Non-native execution of the other three archives remains intentionally unclaimed until native runners execute the smoke entry point.
