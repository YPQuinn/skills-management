# Give the Source page one signal per fact and one primary per state

Type: task
Status: resolved
Blocked by: 11

## Question

After ticket 11 the page read as discordant. Two external reviews landed on the same causes: the move promoted Synchronization without demoting the Inventory, and several facts were being stated twice at full volume.

## Comments

- Two solid primaries shared a viewport: `Synchronize now` at `md` and `Import all` at `sm`. `Import all` stayed primary even on a Source where every entry was already imported, because it is only disabled for `importing`, an empty Inventory, or an unavailable Source. Importing is the primary act only until something is bound.
- A Conflict was shouted twice eight pixels apart: a red `1 Sync conflict` Badge and then a block `Alert` with the same title. The Skill Synchronization tab already picked one signal in `sync-state-panel.tsx`; this page broke its own rule. The Alert wins — it is the one that says how to resolve it.
- A finished run showed `Alert variant="success"` hardcoded, so a run that skipped two of three Skills still claimed success in green while a red Alert sat above it. A toast reports completion without asserting an outcome; the tally line and the per-item results carry the verdict.
- The results `Table` gave four columns to what is now a list; `Detail` was `—` on every row. The Sync Status only explains why something did not move, so it earns a badge only when `isSuccessfulSyncResult` is false.
- One reviewer proposed remapping `conflict` to `warning` in `sync-status-policy.ts`. Rejected: that file is the shared status language for the Skills index (ticket 03), and forking it to fix one page's local redundancy is the wrong lever.
- One reviewer proposed a `Separator` between the two sections. Rejected: once the competing weights are fixed the boundary reads on its own, and a rule would be a band-aid over the real problem.
- The two headings now share a grammar: identity, then the timestamp for that section's reading, then its control. `Last scanned` and `Sync Status last evaluated` sit in the same slot; the longer label keeps the tally from reading as live.
- The allow-large Checkbox left the Inventory heading row for the notes row. It is an import setting, not a heading control.
- Ticket 11's one-line unbound hint floated in the page gutter with a full section's spacing. `SourceSyncSection` now returns `null` and the Inventory carries the sentence in place of its scope note, which has nothing to scope when nothing is imported.

## Answer

The Source page states each fact once and offers one primary per state. `Import all` drops to `outline` as soon as anything is bound, leaving `Synchronize now` as the page's only primary; on an untouched Source the reverse holds. Conflicts speak only through their inline Alert, and settled statuses earn no badge — an all-matching Source gets one sentence instead of a row of green. A finished run reports through the existing `toastSynced` toast, replacing the green banner that used to contradict skipped items, and the outcomes render as a compact list that shows a Sync Status only where the result was not a success.

Ticket 14 then moved the run outcomes off the page entirely: the compact list became the toast's own content, which is where a report about a finished moment belongs, and the Conflict Alert stopped stepping aside for it.

Ticket 13 merged the two sections into one toolbar, which returned `Import all` to `primary`: once the two buttons sit at opposite ends of a single row they read as two secondary-plus-primary groups, not as rival primaries in rival sections. The one-signal rules for Conflict, settled statuses, and run results are unchanged.
