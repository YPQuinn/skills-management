# Compact identity columns and relative timestamps

Type: task
Status: resolved
Blocked by: 05

## Question

Stop repeating Name and Slug when they match, and show relative time in tables with the absolute timestamp available on hover.

## Comments

- Skills index, SkillListPane, and Desired Skill Set currently print name and slug as siblings even when equal.
- Show name as the primary label; show slug only when it differs, as mono secondary text.
- Change `formatTime` (or add `formatRelativeTime` used by list cells) via `Intl.RelativeTimeFormat`. Keep `formatLocaleTime` for titles/tooltips and for dates older than ~30 days.
- Groups index: drop Created/Updated as primary columns if Members already exists; relative Updated is enough.
- Update locale date tests.

## Answer

List identity is name-first; slug appears only when it differs. List cells use `formatRelativeTime` (`<time title>` holds the absolute timestamp). Groups index dropped Created and keeps relative Updated.
