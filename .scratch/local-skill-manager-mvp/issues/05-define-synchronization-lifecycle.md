# Define the synchronization and recovery lifecycle

Type: grilling
Status: resolved
Blocked by: 03

## Question

What exact version identification, change detection, status transitions, conflict actions, atomic update behavior, snapshot retention, and rollback semantics should govern one-way synchronization for Git and Local Sources?

## Context

- [Define the Source and import contract](03-define-source-and-import-contract.md)

## Answer

### Content identity and synchronization baseline

Synchronization uses SHA-256 tree digests rather than Git revisions, mtimes, or directory timestamps as content identity. A digest covers the complete materialized Skill tree: relative paths, node types, file bytes, executable bits, and empty directories. Git commit SHAs, refs, and check times remain provenance evidence.

Each Source Binding retains a **Synchronization Baseline**: the immutable Source content most recently accepted by import, synchronization, exact convergence, or conflict resolution. Checks compare the current Source digest and current Skill Store digest with that baseline:

- Source equals Store: `in_sync`; advance the baseline if both converged independently.
- Source differs from the baseline while Store equals it: `source_changed`.
- Source equals the baseline while Store differs: `store_changed`.
- Both differ from the baseline and from each other: `conflict`.

The baseline includes content needed for three-way differences, not only a digest. It is technical comparison state rather than user-visible version history.

### Source observations

A Git Source check resolves its configured ref into a full commit SHA and inspects an immutable internal checkout. Branches, tags, and the remote default branch are resolved afresh on every check; a full commit SHA remains pinned. Every Source Inventory entry from one check is derived from the same commit. The commit is evidence while each entry's tree digest is its content identity.

A Local Source check ignores mtimes as evidence of equality and performs two consecutive inventory and content scans. The observation is accepted only when paths, node types, sizes, executable bits, and digests agree. An unstable tree receives one short retry; continued change fails the whole Source check and preserves the previous inventory as stale. A local Git working tree registered as a Local Source is treated purely as current filesystem content.

A Source check is the observation boundary: success replaces the Source Inventory as a whole, while failure never installs a partial inventory. Invalid entries may still be reported and skipped according to the Source contract. Synchronization obtains and validates a fresh observation before writing, so it never applies an observation that became stale after an earlier check. A cached Git checkout does not make an unreachable Source eligible for import or synchronization.

### Observable state

Sync Status is the observed content relationship:

- `unbound`
- `unchecked`
- `in_sync`
- `source_changed`
- `store_changed`
- `conflict`
- `source_missing`
- `source_invalid`
- `store_missing`
- `store_invalid`

Source availability and operation health are orthogonal. Each Source records its latest check start and completion times, result, error, observed revision, inventory digest, and latest successful observation time. Each Binding records whether its content relationship is stale. If a Source is unavailable, its last known content relationship is retained and marked stale rather than overwritten with a generic unavailable state.

Each bound Skill also records its latest synchronization action and result, start and completion times, before and after digests, Source revision, and error. Result values are `no_op`, `updated`, `kept_store`, `accepted_source`, `skipped`, `blocked`, and `failed`.

### Safe synchronization and conflict actions

`check` updates observations and comparison state but never rewrites live Skill content. `sync` always begins with a fresh check and uses these rules:

- `in_sync` is a no-op.
- `source_changed` is the only state updated automatically.
- `store_changed` and `conflict` are skipped until the user explicitly chooses an action.
- `source_missing`, `source_invalid`, or an unavailable Source blocks retrieval.
- `store_missing` and `store_invalid` can be repaired only through explicit **Accept Source**.

**Keep Store** leaves live content untouched and accepts the currently observed Source as the new Synchronization Baseline. A conflict consequently becomes `store_changed`; a later Source change becomes a new conflict. **Accept Source** snapshots current Store content, atomically replaces it with validated Source content, and advances the baseline to `in_sync`. **Detach** removes the Source Binding when the user wants the Store copy to become an untracked local Skill. Neither an interactive terminal nor `--yes` implies Accept Source.

Conflicts expose three comparisons: baseline to Source, baseline to Store, and Source to Store. Path entries distinguish additions, deletions, content changes, executable-bit changes, and node-type changes. Ordinary UTF-8 text without NUL bytes can render unified differences; binary, undecodable, or presentation-limit-exceeding files expose paths, sizes, and digests instead. Differences can be requested by path. MVP resolution is whole-Skill only: there is no per-file selection, automatic merge, or editor integration.

### Missing, moved, and invalid entries

A Source Binding continues to identify `Source ID + relative Skill directory`. If that directory disappears, the Skill becomes `source_missing`; its Store content and distributed links remain untouched. A same-name or same-digest entry at another path is never inferred to be a move. The user may wait for the path to return, Rebind explicitly, or Detach.

Rebinding identical content establishes a new baseline and `in_sync`. Different content enters `conflict` without overwriting the Store. An entry that still exists but fails Skill validation becomes `source_invalid` with its validation errors. A whole-Source access failure is availability staleness, not evidence that each entry is missing. A restored entry is compared normally against the retained baseline.

### Atomic replacement and crash recovery

Every Store mutation takes a Store-level exclusive filesystem lock so CLI and WebUI processes cannot modify it concurrently. New content is materialized in an internal staging area on the same filesystem, fully safety-checked, guard-checked, and digest-verified before the live path changes.

A durable operation journal records replacement phases. The existing Skill moves to a recovery slot, staged content is renamed to `<store>/<slug>`, and only then are SQLite baseline, digest, and result fields committed. On success the recovery slot becomes the previous snapshot and the journal is cleared.

Opening the Store or starting another write first resolves any unfinished journal. If SQLite did not commit, recovery restores the old content; if SQLite committed and the live digest matches, recovery completes cleanup. If neither state can be proven valid, writes stop with a recovery error and all candidate content is preserved. Cancellation before commit cleans staging and retains the old state; after commit begins, the operation must finish or run deterministic recovery.

### Snapshot and rollback semantics

Each Skill retains at most one complete **previous snapshot**, distinct from its Synchronization Baseline. Only a successful Store-content replacement rotates this snapshot, including Accept Source, Replace, and repair. Checks, Keep Store, skipped operations, and failed operations do not change it.

The snapshot includes the content tree, digest, Source evidence, reason, and time. Baseline and snapshot content may share content-addressed storage, but have independent roles. The baseline is removed after Detach when no longer referenced; the previous snapshot remains available to that Skill. Neither internal copy participates in Distribution.

Rollback takes the Store lock, rechecks that live content has not changed since confirmation, validates the snapshot, and atomically swaps it into the live path. The pre-rollback live tree becomes the new single previous snapshot, allowing one reverse rollback without accumulating history. Rollback changes only Skill content—not Source Binding, Group, Assignment, or Source state—and then recomputes Sync Status, normally yielding `store_changed`. A missing, corrupt, or unsafe snapshot blocks rollback without changing live content.

### Batches, failure, and retry

Batch synchronization groups Skills by Source so one coherent Source observation serves all its entries, then applies a separate filesystem and SQLite transaction per Skill. One failure never rolls back another Skill's success. The response contains every item outcome plus totals; the durable MVP state retains the latest Source check and latest per-Skill synchronization result rather than an unbounded audit log.

Operations are idempotent and always derive their next action from a fresh state. Apart from the single Local Source stability retry, there are no hidden, background, scheduled, or exponential retries. Network, permission, lock, and recovery failures require an explicit user retry.
