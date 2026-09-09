# Collapse the Source page's two action rows into one toolbar

Type: task
Status: resolved
Blocked by: 12

## Question

Ticket 12 made each section internally coherent but left the page spending four stacked rows — two headings, two timestamps, two description sentences — before the Inventory table began. On a Source whose Skills all match, the whole upper half said nothing actionable.

## Comments

- `Synchronize now` and `Rescan` are both things you do to this Source, and the row that held `Rescan` already held `Import selected` and `Import all`, which write the Store too. The read/write split ticket 10 drew turned out not to be the line the page needed; "acts on this Source" is, and it fits one row.
- The prose moved onto the controls it describes. `Rescan` carries the scope note plus `Last scanned`; `Synchronize now` carries `sourceSyncDesc` plus `Sync Status last evaluated`; both import buttons carry the slug-override note. Four rows of explanation became four Tooltips, and the table now starts around 80px higher.
- `inventoryScopeNote` lost its leading "The Skills this Source offers upstream" — on a button Tooltip the sentence has to describe the button, not the list.
- `Import all` returns to `primary`. With `Synchronize now` at the row's left end and `Import all` at its right, the two read as a pair of groups — a secondary action beside its primary, twice — rather than as two primaries competing for the same glance, which is what ticket 12 was guarding against when they sat in separate sections.
- The section heading `Synchronize bound Skills ({count})` is gone, so `sourceSyncHeading` was deleted from both dictionaries. The bound count already shows in the status line or the badges.
- `SourceSyncSection` split into `useSourceSync`, `SourceSyncButton`, and `SourceSyncStatus`: one POST now drives a control in the toolbar and a status block below it, which a single component cannot do without a portal. `SourceInventory` takes the two as `ReactNode` slots rather than importing the sync API, so Synchronization stays a Source concern that the page composes in.
- The unbound hint moved back into `SourceSyncStatus`, which the Inventory renders in its status slot — same place on screen as ticket 12 put it, but owned by the module whose absence it explains.
- Tooltip content is not covered by tests. Base UI opens on trusted pointer events, which neither jsdom nor the automation harness delivers; the tests pin that the description and the timestamps no longer occupy layout, and the dictionary parity test keeps the copy honest.

## Answer

One toolbar carries every Source action in the order `Rescan`, `Synchronize now`, allow-large, `Import selected`, `Import all`, with each button's purpose and the age of its reading on hover. Below it sits a single status block: the bound Skills' Sync Status, the Conflict Alert, and the last run's outcomes. A Source with nothing bound shows no `Synchronize now` and says why instead.
