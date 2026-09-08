# Sync tab conclusions, conflict Alert, hide Sync on conflict

Type: task
Status: resolved
Blocked by: 06

## Question

Lead the Synchronization tab with a sentence that states the three-way relationship, put Conflict in an Alert that names Keep Store vs Accept Source, collapse digests, and stop offering bare Sync during conflict.

## Comments

- Map `sync_status` to conclusion copy (in sync / Source changed / Store changed / conflict / missing / invalid). Do not ask the operator to compare hex.
- Digests (Store, Baseline, latest action) live in a Collapsible with copyable values.
- Conflict: `Alert` explaining that Synchronization cannot proceed until Keep Store or Accept Source. Tooltips on Baseline if needed.
- `syncActionsFor`: conflict must not include `sync`. Keep Store and Accept Source stay behind their existing named Alert Dialogs.
- Update `sync-policy.test.tsx` and `skill-sync.test.tsx` (conflict no longer clicks Sync).

## Answer

The Synchronization tab leads with a status sentence, or a conflict Alert that names Keep Store vs Accept Source. Digests sit in a Collapsible. `syncActionsFor` no longer offers Sync while status is conflict.
