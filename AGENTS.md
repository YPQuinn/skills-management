## Agent skills

### Issue tracker

Issues are tracked as local Markdown files under `.scratch/`. See `docs/agents/issue-tracker.md`.

### Domain docs

This repository uses a single-context domain documentation layout with its glossary at `docs/CONTEXT.md`. See `docs/agents/domain.md`.

## Delegation

Never use `pi-subagents` for delegated work in this repository. Use Herdr-managed agents directly in the primary checkout; do not create a worktree unless the user explicitly requests one. Research tasks must use a Herdr-managed Pi agent with `xai/grok-4.5` at `high` thinking unless the user explicitly overrides it.

## Appica UI

All WebUI page styling and UI components **must use Appica**. Do not introduce another component library or hand-build a substitute when Appica provides the required component or pattern.

Before implementing or reviewing any WebUI work:

1. **First read `docs/design/llms.md`** to locate the appropriate Appica component.
2. **Then search `docs/design/llms-full.md` by component name or keyword** for its exact API and usage. Do not guess component APIs or read the full file linearly.
3. Use the live [Appica UI documentation](https://appica.dev/ui/docs) as an additional reference when the local documentation is insufficient.

## Rules

Follow rules in `docs/rules/anti-overengineering.md`