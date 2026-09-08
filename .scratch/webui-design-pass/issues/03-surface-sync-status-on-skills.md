# Surface Sync Status on Skills index and list pane

Type: task
Status: resolved
Blocked by: 02

## Question

Show the Sync Status the Skills list API already returns, so the operator can see drift and conflicts without opening every Skill.

## Comments

- `Skill.sync_status`, `sync_stale`, and `last_sync` are already on `/api/v1/skills`. Reuse `SyncStatusBadge`.
- Skills index: replace or add a Sync column; keep search. Add clickable counts (in sync / source changed / conflict / unbound / stale) that filter the table. No new page.
- `SkillListPane`: a status color is not enough — keep a text badge or aria-label from the same status mapping.
- Extend `skills.test.tsx` with a conflict fixture visible on the index.

## Answer

Skills index shows Sync Status (and stale) from `/api/v1/skills`, with Chip filters for in sync / Source changed / Store changed / conflict / unbound / stale. SkillListPane shows the same status badge.
