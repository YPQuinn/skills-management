# Three-way diff by path with result language

Type: task
Status: resolved
Blocked by: 07

## Question

Aggregate the three comparisons into one row per path with Upstream / Store chips in result language, and collapse empty comparisons into a single empty state.

## Comments

- Current UI repeats the same file under Baseline→Source, Baseline→Store, and Source→Store. `Source→Store` + `added` on the other side reads as “deleted” for an upstream-only file.
- One list keyed by path. Chips: upstream added/removed/changed/unchanged and store added/removed/changed/unchanged, derived from the three comparisons.
- Expand a path (Appica Accordion) to show the relevant unified diffs, labeled in result language, not “Baseline → Source”.
- If every comparison is empty, one line: no differences. Drop the three identical empty cards.
- Update `skill-sync-diff.test.tsx`.

## Answer

Diffs are one Accordion row per path with Upstream/Store chips. Expanded panels use Upstream / Store / Source vs Store labels. An all-empty comparison set shows a single “No differences.” line.
