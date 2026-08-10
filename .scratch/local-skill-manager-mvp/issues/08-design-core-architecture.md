# Design the core architecture and REST boundary

Type: grilling
Status: resolved
Blocked by: 02, 03, 04, 05, 06, 07

## Question

How should Go packages, filesystem ownership, SQLite state, synchronization and Distribution services, REST/JSON resources, chi serving, and the embedded Appica application be separated so CLI and WebUI share one reliable core—including alignment of the Vite asset base, client-router basename, chi mount and SPA fallback, REST prefix, theme policy, and any CSP?

## Context

- [Research Appica's embedded-WebUI constraints](02-research-appica-embedded-webui.md)

## Answer

### Architecture shape

Skill Manager is one modular Go application, not a daemon plus clients. Cobra commands and chi handlers call the same in-process `app.App`; the CLI never loops back through HTTP. Only `skillctl ui` starts the localhost HTTP server. Application methods are semantic commands and queries such as `AddSource`, `ImportSkills`, `SyncSkill`, `InspectTarget`, and `DistributeTarget`, with explicit request/result types and stable typed errors. Repositories and generic CRUD are not exposed to adapters.

The package layout is capability-oriented:

```text
cmd/skillctl            executable composition root
internal/bootstrap      uninitialized/initialized transition
internal/domain         shared entities, IDs, statuses, and pure invariants
internal/app            use-case facade, orchestration, and transaction boundaries
internal/source         Source normalization, observation, and materialization
internal/skillstore     authoritative Store trees and atomic replacement primitives
internal/sync           Synchronization state machine, diff, resolution, and rollback
internal/target         Target Adapter table, detection, and path resolution
internal/distribution   desired set, inspection, plan, reconciliation, and adoption
internal/state          SQLite persistence and migrations
internal/cli            Cobra adapter
internal/httpapi        REST/JSON and chi routing
internal/webui          Appica SPA source, embedded build output, and static handler
```

`internal/domain` is deliberately small: it contains only glossary concepts shared across capabilities and has no SQL, JSON, HTTP, or filesystem dependencies. Capability-specific observations, plans, diffs, and request/result types stay with their owning package. `app` composes concrete implementations and coordinates cross-capability work but does not absorb the synchronization or Distribution rules.

Interfaces exist only at real variation points or narrow boundaries. Local and Git Sources implement one small observation/materialization contract. GitHub shorthand remains input normalization for Git rather than a third Source kind. Target Adapters are a compiled data table plus pure resolver/detector functions; custom Targets are data, not plugins. There is no dynamic plugin system or promised third-party Go API.

### Ownership and persistence

`state` is the only package that executes SQL. It uses `database/sql` with `modernc.org/sqlite`, normalized relational tables, foreign keys, and uniqueness constraints. It stores current entities and relationships, current observations, Synchronization Baseline references, the single previous-snapshot reference, Managed Link ownership, latest operation outcomes, and unfinished operation intents. It does not store Skill trees as blobs, implement event sourcing, or accumulate an unbounded audit log.

Schema migrations are numbered SQL embedded in the executable and applied in order when the database opens. A database newer than the executable is rejected clearly. Pure metadata changes use one SQLite transaction. SQLite runs in WAL mode with a finite busy timeout, but database locking is not treated as filesystem mutual exclusion.

SQLite assigns non-reused `int64` IDs to Source, Skill, Group, and Target records. REST uses these IDs for resource actions and relationships. CLI commands and readable browser routes continue to use slugs and operator-facing names, which the application resolves to IDs. Renaming therefore preserves identity and relationships while producing a new readable browser URL.

Filesystem ownership is exclusive:

- `skillstore` alone reads and mutates authoritative Store content and its internal working trees.
- `source` alone reads Local Sources and manages Git observations/checkouts.
- `distribution` alone inspects or mutates registered Target entries.
- CLI and HTTP adapters never execute SQL or perform domain filesystem mutations directly.

The Store layout reserves an internal directory on the same filesystem as live Skills:

```text
<skill-store>/
  <skill-slug>/
  .skillctl/
    staging/
    baselines/<skill-id>/
    previous/<skill-id>/
    recovery/
```

This directory is excluded from Skill enumeration, import, and Distribution. Baseline and previous-snapshot trees remain separate ordinary directory copies in the MVP; there is no content-addressed object store, deduplication, or reference counting. Same-filesystem staging and recovery preserve atomic rename/swap behavior. Git cache and lock files live beside `state.db`, not in the Store or Targets.

Filesystem and SQLite changes are coordinated explicitly rather than presented as one transaction. A durable operation intent is recorded first, filesystem preconditions and mutations run second, and metadata is finalized last. Physical recovery slots preserve candidate content. Every process resolves unfinished Store journals and Distribution intents before its next write.

Cross-process file locks coordinate simultaneous CLI and WebUI processes. Store replacement takes the Store exclusive lock. Distribution takes the Store shared lock and one Target exclusive lock; multi-Target operations acquire Target locks in stable Target-ID order. Reads may run concurrently. Lock failures surface immediately for explicit retry—there are no hidden retries.

### Bootstrap and configuration

Initialization has a separate narrow boundary so normal application methods never operate with partially constructed dependencies. `bootstrap` discovers whether configuration exists, performs Target Adapter detection, and exposes one Initialize use case shared by `skillctl init` and `/setup`. The WebUI collects its Storage and optional Target choices client-side and submits them once; successful initialization atomically establishes configuration, Store, SQLite, and selected Target registrations, then constructs the complete `app.App` without restarting the HTTP server. Before that transition, only setup/status API operations are available; ordinary operations return `not_initialized`.

`config.toml` contains only the absolute Skill Store path, absolute `state.db` path, and, if needed, its own format version. Sources, Targets, Groups, Assignments, and operational state belong in SQLite. Lock and Git-cache locations derive from the state database directory. Runtime options such as `--port` and `--no-open` are not persisted. Target Adapter environment overrides are read only during detection or Target creation. Configuration is written with a temporary file and atomic rename, without a separate backup-history system.

### Synchronization and Distribution services

`sync` owns three-way status evaluation, Source observation inputs, diff generation, safe-update planning, conflict actions, journal-aware replacement, and rollback semantics. `distribution` owns desired-set expansion, coherent inspection, no-overwrite plans, Managed Link proof, adoption, durable link intents, and reconciliation outcomes. `skillstore` provides the lower-level safe tree operations used by synchronization; neither adapter duplicates these rules.

All operations are synchronous. CLI and REST wait for the same application call and receive its complete result. Request cancellation is honored before commit; once an operation enters its non-interruptible commit phase, it completes or leaves a deterministic recovery intent. The WebUI shows an Appica loading/progress state. The MVP has no job table, poll API, SSE, WebSocket, or background worker. Batch operations return every item outcome and a summary after completion.

### REST/JSON boundary

REST is versioned at `/api/v1` and uses pragmatic resources plus explicit domain actions rather than forcing every operation into generic CRUD. Collections and details exist for Sources, Skills, Groups, and Targets; Assignments and Group memberships are nested relationships. Representative actions include:

```text
POST /api/v1/sources/{id}/check
POST /api/v1/skills/import
POST /api/v1/skills/{id}/sync
POST /api/v1/skills/{id}/rollback
POST /api/v1/targets/{id}/inspect
POST /api/v1/targets/{id}/distribute
POST /api/v1/targets/{id}/adopt
```

Conflict choices such as `keep_store` and `accept_source` are explicit request-body actions, not generic state patches. API models map application results into JSON and never expose SQLite rows or internal Store paths accidentally.

JSON fields and enum values use `snake_case`; timestamps use UTC RFC3339. A single resource is returned directly, while collections use `{ "items": [...], "total": n }`. The local MVP does not paginate. Errors use a stable shape:

```json
{
  "error": {
    "code": "target_conflict",
    "message": "Target entry cannot be replaced safely",
    "details": {}
  }
}
```

HTTP status codes distinguish malformed input, missing resources, state conflicts, and unexpected failures. Once a batch request is valid and begins processing, it returns HTTP 200 with per-item outcomes rather than `207 Multi-Status`; top-level 4xx/5xx responses mean the request itself could not run. CLI `--json` preserves the same result fields and error codes without copying the HTTP envelope.

### HTTP and embedded Appica application

The production URL topology is fixed rather than runtime-configurable:

```text
WebUI / client router: /
Vite base:             /
BrowserRouter basename:/
REST:                  /api/v1
Vite assets:           /assets/*
```

This preserves `/setup`, `/skills`, `/sources/...`, `/groups/...`, and `/targets/...` as readable History API routes. chi registers `/api/v1/*` before the UI handler; an unknown API route always receives a JSON 404. The UI handler first serves an exact embedded file. Remaining valid `GET`/`HEAD` client routes fall back to `index.html`. Missing `/assets/*` paths and missing paths with file extensions return 404 rather than receiving the SPA shell.

The WebUI is a React 19 + Tailwind v4 Vite SPA using Appica exclusively. It imports `@appica/ui-react/styles.css`, registers the pinned Appica `dist` path with Tailwind `@source`, uses component subpath imports, composes navigation onto the chosen router, and wraps the root in Appica `ThemeProvider`. It uses Appica's default tokens and system fonts; no second UI kit or custom brand-theme layer is added.

Vite source and build output live under `internal/webui/`, with production output at `internal/webui/dist`. `//go:embed all:dist` places that compiled `index.html` and hashed assets into the Go executable. Production builds run the frontend build before `go build`; running `skillctl ui` requires no Node installation. Development uses a separate Vite process that proxies `/api/v1` to chi; Go does not implement a development proxy or hot reload. Whether generated `dist` is committed and the exact release command remain part of packaging acceptance.

Theme policy is `light`, `dark`, and `system`, defaulting to `system`. Appica owns persistence in browser `localStorage`; theme state does not enter SQLite or REST. Reduced-motion behavior follows the platform and Appica.

The production chi handler sends this CSP, intentionally allowing Appica's documented inline theme script and component inline styles rather than adding dynamic nonce injection:

```text
default-src 'self';
script-src 'self' 'unsafe-inline';
style-src 'self' 'unsafe-inline';
img-src 'self' data:;
font-src 'self';
connect-src 'self';
object-src 'none';
base-uri 'none';
frame-ancestors 'none';
form-action 'self'
```

It also sends `X-Content-Type-Options: nosniff` and a strict Referrer Policy. No remote fonts, CDNs, or third-party connections are allowed.

`skillctl ui` listens only on `127.0.0.1`; the MVP has no external bind option. The server validates the Host as loopback, sends no permissive CORS headers, requires `application/json` for REST mutations, and rejects a supplied Origin that does not exactly match the current loopback origin. GET operations remain read-only. With no cookies or sessions, the MVP needs neither accounts nor a separate CSRF-token system.
