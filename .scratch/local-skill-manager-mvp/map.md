# Deliver the runnable Skill Manager MVP

Label: wayfinder:map

## Destination

Deliver a runnable, local-first Skill Manager MVP for macOS and Linux, preferably as one `skillctl` executable with functionally equivalent CLI and Appica WebUI. It manages an authoritative Skill Store, upstream Sources, overlapping Groups, and safe symlink Distribution to Agent Targets with observable sync and distribution state.

## Notes

- This effort explicitly carries execution through the map: the destination is a validated working MVP, not only a specification.
- Before each session, read `AGENTS.md` and `docs/CONTEXT.md`; use `/grilling` and `/domain-modeling` for unresolved decisions.
- Follow `AGENTS.md` for delegation: research uses Herdr-managed Grok in a `research` tab, visual/UI work uses Herdr-managed AGY with Gemini 3.1 Pro in a `uiux` tab, and other work uses `pi-subagents`; delegated agents report back to the sender. Work in the primary checkout and do not create worktrees unless explicitly requested.
- The product is single-machine, single-user, and local-first. The WebUI listens only on localhost; accounts, authorization, remote service, and collaboration are excluded.
- Product name: **Skill Manager**. CLI command: `skillctl`.
- Core and CLI: Go + Cobra. Local HTTP service: Go + chi. State: SQLite through `modernc.org/sqlite`.
- WebUI: Appica only, calling the Go core through REST/JSON. Prefer embedding Appica build output with Go `embed` into one executable.
- All WebUI styling and components must use Appica. First consult `docs/design/llms.md`, then search `docs/design/llms-full.md` by component or keyword; use https://appica.dev/ui/docs as an additional reference.
- The default Skill Store is `~/.skillctl/store`, configuration is `~/.skillctl/config.toml`, and state is `~/.skillctl/state.db`; `skillctl init` may select another absolute Store path.
- A Skill is an Agent Skills directory containing `SKILL.md`, identified by a globally unique slug. Sources are upstream only; consumers link exclusively to the Skill Store.
- MVP Sources must include GitHub and local paths. GitHub authentication reuses existing Git/SSH/HTTPS or `gh` credentials; Skill Manager never stores tokens.
- Each Skill binds to at most one Source; one Source may expose multiple Skills. A binding can be removed or replaced without deleting the Skill.
- Groups are overlapping, non-owning sets. A Target may receive Groups and individual Skills; their union is its desired Skill set.
- Distribution never overwrites unmanaged files, directories, or links. Only links managed by Skill Manager may be changed or removed.
- Distribution tracks desired state, observed state (`linked`, `missing`, `conflict`, `broken link`), and the latest reconciliation result.
- Synchronization is manual and one-way from Source to Skill Store. It tracks upstream changes, Store changes, conflicts, source availability, checks, and results.
- Sync conflicts are explicit: view file differences, keep Store content, or accept Source content. Batch sync skips conflicts.
- Sync writes use temporary content and atomic replacement, retaining one previous snapshot for automatic recovery and one-step manual rollback.
- Deleting a referenced Skill is blocked by default; explicit deletion first removes related Assignments and managed links.

## Decisions so far

<!-- One linked gist is appended here for each resolved child ticket. -->

- [Research Vercel Skills compatibility surface](issues/01-research-vercel-skills-compatibility.md) — Captured the commit-specific six-Source compatibility surface, discovery rules, 76-Agent path matrix, installation behavior, ambient authentication, and lock/update models that the MVP must deliberately select from.
- [Research Appica's embedded-WebUI constraints](issues/02-research-appica-embedded-webui.md) — Established the React 19, Tailwind v4, Appica package and theming contract while isolating Vite asset paths, client routing, SPA fallback, and Go embedding as architecture decisions.
- [Define the Source and import contract](issues/03-define-source-and-import-contract.md) — Chose structured Local/Git Sources, deterministic depth-three inventories, explicit full-tree atomic imports, safe slug replacement, and detachable path-based Source Bindings.
- [Define the Target adapter contract](issues/04-define-target-adapter-contract.md) — Chose eight path-based built-in adapters, safe fixed-path custom Targets, physical-path identity, and advisory on-demand Agent detection.
- [Define the synchronization and recovery lifecycle](issues/05-define-synchronization-lifecycle.md) — Chose SHA-256 three-way state, conservative explicit conflict actions, stable Source observations, crash-recoverable atomic replacement, and one-step rollback.
- [Define the Distribution reconciliation lifecycle](issues/06-define-distribution-lifecycle.md) — Chose Assignment-derived desired sets, ledger-proven absolute symlinks, two-dimensional observed state, no-overwrite reconciliation, explicit adoption, and crash-safe cleanup.
- [Prototype the Skill Manager operator workflow](issues/07-prototype-operator-workflow.md) — Chose an Appica resource-explorer WebUI, responsive master–detail routes, separate status dimensions, and a matching noun-first `skillctl` command language.
- [Design the core architecture and REST boundary](issues/08-design-core-architecture.md) — Chose an in-process modular core with exclusive filesystem and SQLite ownership, recoverable cross-process writes, synchronous versioned REST, and a root-mounted embedded Appica SPA.
- [Define MVP packaging and acceptance](issues/09-define-mvp-acceptance.md) — Set four native single-binary releases, deterministic offline quality gates, CLI and browser journeys, crash/concurrency/state-loss recovery, and documentation plus public-GitHub final validation.
- [Establish the runtime and bootstrap foundation](issues/10-establish-runtime-bootstrap-foundation.md) — Delivered the tested Go/bootstrap/SQLite/locking runtime, secure Cobra/chi setup boundary, and embedded Appica shell with restart and deep-link support.
- [Implement Source registration and inventory](issues/11-implement-source-registration-inventory.md) — Delivered stable Local/Git observation, safe ambient GitHub authentication, canonical Inventory digests, transactional Source state, noun-first CLI and strict REST operations, and the Appica Source explorer.
- [Implement Skill import and Store materialization](issues/12-implement-skill-import-store.md) — Delivered safe full-tree imports and replacements, durable identity-bound Store recovery receipts, Source Bindings and per-item outcomes, matching noun-first CLI and strict REST contracts, and Appica Skills plus Source Inventory import workflows.
- [Add English and Simplified Chinese WebUI localization](issues/20-add-webui-localization.md) — Added a typed dependency-free locale layer, persisted Appica language switching, localized WebUI copy/accessibility/timestamps/client fallbacks, and no-refetch locale transitions while preserving server messages verbatim.

## Not yet specified

<!-- No remaining in-scope fog. -->

## Out of scope

- Windows support.
- Remote or multi-user operation, accounts, permissions, and collaboration.
- Background daemon, scheduled synchronization, and filesystem watching.
- Publishing or pushing Skill Store changes back to a Source.
- Full version history beyond the immediately previous synchronization snapshot.
- Non-Agent-Skills content such as standalone prompts, MCP servers, and general plugins.
- Automatic overwrite, deletion, or adoption of unmanaged Target content.
