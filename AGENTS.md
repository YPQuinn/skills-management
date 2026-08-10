## Agent skills

### Issue tracker

Issues are tracked as local Markdown files under `.scratch/`. See `docs/agents/issue-tracker.md`.

### Domain docs

This repository uses a single-context domain documentation layout with its glossary at `docs/CONTEXT.md`. See `docs/agents/domain.md`.

## Delegation

When using `pi-subagents`, if the scene is researching / websearching, use `herdr` to launch `grok-build` + `grok-4.5` + `high` as a subagent in new a tab(tab name is 'research'). If the scene is visaul engineering / UIUX designing and implementing , use `herdr` to launch `agy` (`antigravity-cli`) + `gemini-3.1-pro` + `high` as a subagent in a new tab(tab name is 'uiux'). If a subagent is delegated by this way, tell him to reply to sender when finished his job. For other scenes, just use `pi-subagents` to delegate subagents inside `pi` session.

## Appica UI

All WebUI page styling and UI components **must use Appica**. Do not introduce another component library or hand-build a substitute when Appica provides the required component or pattern.

Before implementing or reviewing any WebUI work:

1. **First read `docs/design/llms.md`** to locate the appropriate Appica component.
2. **Then search `docs/design/llms-full.md` by component name or keyword** for its exact API and usage. Do not guess component APIs or read the full file linearly.
3. Use the live [Appica UI documentation](https://appica.dev/ui/docs) as an additional reference when the local documentation is insufficient.

## Rules

Follow rules in `docs/rules/anti-overengineering.md`