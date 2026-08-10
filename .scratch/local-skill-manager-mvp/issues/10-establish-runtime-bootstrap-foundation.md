# Establish the runtime and bootstrap foundation

Type: task
Status: resolved
Blocked by: 09

## Question

Implement the runnable foundation selected by [Design the core architecture and REST boundary](08-design-core-architecture.md): Go composition root, domain and application boundaries, configuration, SQLite migrations, bootstrap/initialization, cross-process locks, Cobra/chi entry points, `/api/v1` setup boundary, and the root-mounted embedded Appica shell. Prove that `skillctl init`, uninitialized `skillctl ui`, initialization through `/setup`, restart, SPA deep links, API isolation, and loopback security work through both CLI and browser-facing seams.

## Answer

Implemented the runnable Go and Appica foundation: an application/bootstrap boundary with typed errors and explicit `uninitialized`, `ready`, `state_missing`, and `invalid` states; atomic TOML configuration; embedded numbered SQLite migrations; non-blocking shared/exclusive process locks; and Cobra `init`, `status`, `ui`, and version seams. The chi service now supports in-process setup through `/api/v1`, exact loopback Host and Origin enforcement, stable JSON errors, CSP/security headers, API isolation, and exact-file plus safe SPA fallback behavior.

The embedded React 19, Tailwind v4, Appica-only shell provides setup, state-error surfaces, persistent system/light/dark theme selection, responsive resource navigation, and list/detail deep-link foundations. Hermetic Go, separate-process CLI/UI, HTTP contract, lock/concurrency, migration, static-serving, and Vitest suites cover initialization, custom Store paths, restart, setup transition, state loss, nested and trailing-slash deep links, assets, API 404s, and security boundaries. Final validation passed frontend lint/tests/build, `go vet ./...`, `go test ./...`, `go test -race ./...`, module tidiness, and whitespace checks.
