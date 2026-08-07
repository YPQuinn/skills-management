# Deliver the runnable Skill Manager MVP

Label: wayfinder:map

## Destination

Deliver a runnable, local-first Skill Manager MVP for macOS and Linux, preferably as one `skillctl` executable with functionally equivalent CLI and Appica WebUI. It manages an authoritative Skill Store, upstream Sources, overlapping Groups, and safe symlink Distribution to Agent Targets with observable sync and distribution state.

## Notes

- This effort explicitly carries execution through the map: the destination is a validated working MVP, not only a specification.
- Before each session, read `AGENTS.md` and `docs/CONTEXT.md`; use `/grilling` and `/domain-modeling` for unresolved decisions.
- Never use `pi-subagents` for delegation. Use Herdr-managed agents directly in the primary checkout and do not create worktrees unless explicitly requested; research tasks use Pi with `xai/grok-4.5` at `high` thinking unless explicitly overridden.
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

## Not yet specified

- The implementation sequence and vertical slices will become specifiable after the source, target, lifecycle, experience, and architecture decisions are resolved.
- End-to-end hardening, release checks, and user onboarding artifacts depend on the implemented shape and validation contract.

## Out of scope

- Windows support.
- Remote or multi-user operation, accounts, permissions, and collaboration.
- Background daemon, scheduled synchronization, and filesystem watching.
- Publishing or pushing Skill Store changes back to a Source.
- Full version history beyond the immediately previous synchronization snapshot.
- Non-Agent-Skills content such as standalone prompts, MCP servers, and general plugins.
- Automatic overwrite, deletion, or adoption of unmanaged Target content.
