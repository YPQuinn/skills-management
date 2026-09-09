# Report a Synchronization run in its toast, not in the page

Type: task
Status: resolved
Blocked by: 13

## Question

Ticket 12 put a run's outcomes in a compact list under the Source's Synchronization controls. On a Source with 37 Bindings that list is 37 rows long — a batch that changed one Skill still pushed the Inventory table off the screen, and the rows stayed there until the next navigation.

## Comments

- A run's outcomes describe a moment that has already passed; the page's job is to show what the run left behind. So `useSourceSync` no longer keeps `result` in state at all — it fires a toast and lets `onSynced` refresh the Skills, which is where the durable truth already lived.
- The Appica docs type `add`'s `title` and `description` as `string`; the shipped types say `ReactNode`. Confirmed against the installed package before designing around it, so the toast carries real `Link`s and Badges rather than a flattened sentence.
- The toast names only the Skills a run left for the reader — `skipped`, `blocked`, `failed`. `isSuccessfulSyncResult` was the wrong predicate to reuse: it reports `no_op` as unsuccessful, but a no-op asks nothing of anyone. `syncResultNeedsAttention` in `sync-status-policy.ts` draws the line the toast needs, next to the existing one rather than in a component.
- `batchSyncSummary` printed every bucket, so a clean run read `0 kept, 0 accepted, 0 blocked, 0 failed, 0 rolled back` — three wrapped lines of zeros in a 360px toast. Non-zero buckets now render as counted `SyncResultBadge`s, reusing the counted-badge pattern the page's Sync Status already uses. The key was deleted; nothing else referenced it.
- Per-row `SyncResultBadge` dropped: the tally counts the results, and the row's job is to say which Skill and why. Keeping both would restate the same fact eight pixels apart, which is what ticket 12 was about.
- A toast asking the reader to act cannot expire while they read it, so `timeout: 0` when anything needs attention. Verified it outlives the 5s default; a clean run keeps the default and dismisses itself.
- Capped at five named Skills. A Source with dozens of Bindings can strand dozens at once, and a toast tall enough for all of them covers the page; `batchSyncMoreItems` points at the Skills index for the rest.
- The Conflict Alert lost its `result === null` guard. It stepped aside for the results list under ticket 12; with the list gone it is once again the page's standing Conflict signal, and it now survives a run that could not clear it.
- The toast link's `decoration-border` was `oklch(0.928 …)` against the toast's own light surface — an underline nobody could see. Checked the computed style rather than the class name; the links now take the foreground colour.

## Answer

`Synchronize now` reports through its toast: counted result badges for the run, then up to five named Skills that still need the reader, each linking to its Synchronization tab. The toast waits until dismissed whenever it names anyone. The page keeps only what outlasts the run — the bound Skills' Sync Status, and the Conflict Alert that no longer hides after a failed attempt to clear it.
