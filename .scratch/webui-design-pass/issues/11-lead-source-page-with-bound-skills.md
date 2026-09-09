# Lead the Source page with the bound Skills, not the upstream Inventory

Type: task
Status: resolved
Blocked by: 10

## Question

Batch Synchronization sits below a paginated Inventory, so the page's only recurring task and its per-item outcomes are both below the fold. Move the section above the Inventory and make it lead with the bound Skills' Sync Status instead of a bare button.

## Comments

- Importing is a one-time act per Skill; synchronizing is the recurring reason to reopen a Source. Ordering the recurring task under the one-time one is backwards, and it matches this pass's own "registered data first" call in ticket 05.
- Order becomes header, alerts, facts, Synchronization, Inventory, issues. Invalid entries then sit directly under the Inventory they were excluded from.
- `GET /api/v1/skills` is already fetched by this page and carries `sync_status` and `sync_checked_at`, so the summary needs no request and no backend change.
- `summarizeBoundSync` reports the *oldest* evaluation among bound Skills and gives up entirely if one was never evaluated. A rescan does not re-evaluate Sync Status, so the badges must not read as live.
- Status order puts what needs a human decision first, then what a batch run can fix, then the quiet ones. Conflicts also raise the ticket 07 Alert naming Keep Store and Accept Source.
- `SyncStatusBadge` takes an optional `count`; both dictionaries put the number first, as the ticket 03 filter chips already do.
- Zero Bindings collapses to one muted line with no heading and no disabled button — a fresh Source should not be greeted by a dead section above its Inventory. `sourceSyncNoneBound` now points at the Inventory *below*.

## Answer

Synchronization now sits between the Source facts and the Inventory, and it opens with a Sync Status tally of the bound Skills rather than a lone button — decision-needing statuses first, dated by the oldest reading so the badges cannot pass for live. Conflicts raise the Alert naming Keep Store and Accept Source. Per-item outcomes land in view instead of at the page bottom, and a run refreshes the tally in place. A Source with no Bindings says so instead of heading a section with a disabled button.

Ticket 12 then tuned what this move exposed: which statuses earn a badge, where the unbound sentence lives, and how a finished run reports.
