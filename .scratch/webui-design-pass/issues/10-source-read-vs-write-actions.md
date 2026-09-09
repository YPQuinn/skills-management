# Separate the Source page's read action from its write action

Type: task
Status: resolved

## Question

On Source detail, "Check again" and "Sync bound Skills" read as synonyms. Split them by what they change: rescanning replaces the Inventory, Synchronization writes the Skill Store. Move each action next to the object it affects, name the scope, and give the rescan a visible result.

## Comments

- `SyncSkills` already calls `observeSource` first, so Synchronization contains the check. The two buttons are not peers, and the copy must say so instead of implying a two-step flow.
- Registration already says "Register and scan" / "Scanning…", so scan is the established word for observing a Source. Move the Source surface onto it: `btnRescan`, `alertRescanFailed`, `errRescanningSourceFailed`, `colLastScanned`.
- Move the action out of `SourceDetailHeader` and into the Inventory heading row it refreshes, as `variant="outline" size="sm"` with `Last scanned: <RelativeTime>` beside it. The header keeps identity, status, and delete.
- Drop the absolute timestamp from the `SourceFacts` summary line; it stays in the expanded details. Two formats of one timestamp within a screen defeats the point.
- Add a scope sentence under the Inventory heading stating that a rescan never changes imported Skills.
- `SourceSyncSection` takes the bound count already computed for `boundSkillMap`: heading carries the count, button is a bare `Synchronize now`, and it is disabled with an explanation at zero instead of returning an empty batch.
- A Source check does not re-evaluate per-Skill `sync_status`. The rescan toast may report Inventory add/remove/change only; it must not claim Skills have upstream updates.
- Keep the glossary terms. Synchronization is not renamed.

## Answer

The Source page now splits by what each action changes. Rescan sits in the Inventory heading row with `Last scanned` beside it and a sentence stating that it never touches imported Skills; the header keeps only identity, status, and delete. Synchronization carries the bound count in its heading, describes itself as writing the Store after an automatic rescan, and disables itself with an explanation when nothing is bound. A rescan reports its own Inventory add/remove/change tally as a toast through `diffInventory`. The Source surface speaks one verb: `btnRescan`, `alertRescanFailed`, `errRescanningSourceFailed`, `colLastScanned`.

Ticket 13 later dropped the read/write split itself: the row holding Rescan already held the import buttons, so all four became one "acts on this Source" toolbar, and the scope sentence and `Last scanned` moved onto the Rescan Tooltip. The vocabulary and the rescan tally stand.
