# Improve WebUI information architecture and operator scan

Label: wayfinder:map
Status: resolved

## Destination

Make the localhost WebUI answer “what needs attention?” from the list pages, keep create/register off the first screen, and make Synchronization speak in conclusions rather than raw digests and three stacked diffs. Stay on Appica. Do not invent a dashboard page.

## Notes

- Read `docs/design/llms.md`, then search `docs/design/llms-full.md` and `docs/design/icons.md` before adding a component or icon.
- Use glossary terms from `docs/CONTEXT.md`. Do not rename Synchronization, Distribution, Source Binding, Baseline, or Managed Link.
- Prefer existing REST fields. Ticket 04 may expose already-stored Target Distribution outcome fields on the list DTO; it must not add a new backend resource.
- English and Simplified Chinese dictionaries stay in lockstep.
- Chromium/Firefox/WebKit journeys in `e2e/webui` must keep working after create forms move into dialogs.

## Decisions so far

- [Fix dark-mode Skill tab contrast](issues/01-fix-dark-mode-tab-contrast.md) — Removed `!text-black` so tabs use Appica foreground tokens.
- [Remove dead Skill Distribution tab](issues/02-remove-dead-skill-distribution-tab.md) — Distribution stays on Target detail only.
- [Surface Sync Status on Skills](issues/03-surface-sync-status-on-skills.md) — Skills index and list pane show Sync Status plus filter chips.
- [Surface Distribution health on the Targets index](issues/04-surface-distribution-on-targets-index.md) — List DTO exposes stored last_result and stale; no Inspect on list.
- [Invert list-page IA](issues/05-invert-list-page-create-dialogs.md) — Create/register forms moved into header Dialogs; adapter detection sits inside Register Target.
- [Compact identity and relative time](issues/06-compact-identity-and-relative-time.md) — Name-first identity; relative timestamps in list cells.
- [Sync conclusions and conflict Alert](issues/07-sync-conclusions-and-conflict-alert.md) — Sentence/Alert lead; no Sync during conflict; digests collapsed.
- [Path-aggregated diff](issues/08-path-aggregated-diff.md) — One Accordion row per path with Upstream/Store chips.
- [Appica polish](issues/09-appica-polish.md) — Skeleton, Copy, term tooltips, success toasts, Combobox assignment pickers, quieter Delete, visible nav overflow.

## Not yet specified

- Whether a Skill-side Distribution tab should later list Targets that currently desire that Skill.

## Out of scope

- A new home/dashboard route.
- Backend changes beyond exposing already-stored Target list health fields.
- Scheduled Synchronization, filesystem watching, or push-to-Source.
- Replacing Appica or adding another component library.
