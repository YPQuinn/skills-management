-- Synchronization comparison state and outcomes (decision 05): each Skill
-- records its observed Sync Status relationship, its staleness, the latest
-- check time, and the latest synchronization action with start/completion
-- times, before/after content digests, the observed Source revision, and
-- the failure detail. The status set is the decision-05 closed list; the
-- baseline journal column distinguishes ordinary replacements (finalize
-- installs the operation's Baseline candidate) from rollbacks (finalize
-- retains the existing Baseline, so the Skill becomes store_changed).
ALTER TABLE skills ADD COLUMN sync_status TEXT NOT NULL DEFAULT 'unchecked'
    CHECK (sync_status IN ('unchecked', 'unbound', 'in_sync', 'source_changed',
        'store_changed', 'conflict', 'source_missing', 'source_invalid',
        'store_missing', 'store_invalid'));
ALTER TABLE skills ADD COLUMN sync_stale INTEGER NOT NULL DEFAULT 0;
ALTER TABLE skills ADD COLUMN sync_checked_at TEXT;
ALTER TABLE skills ADD COLUMN last_sync_action TEXT NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN last_sync_result TEXT NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN last_sync_started_at TEXT;
ALTER TABLE skills ADD COLUMN last_sync_completed_at TEXT;
ALTER TABLE skills ADD COLUMN last_sync_before_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN last_sync_after_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN last_sync_revision TEXT NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN last_sync_error TEXT NOT NULL DEFAULT '';

ALTER TABLE store_operations ADD COLUMN baseline TEXT NOT NULL DEFAULT 'advance'
    CHECK (baseline IN ('advance', 'keep'));

-- The expected pre-operation Baseline digest of a replace whose displaced
-- Baseline differs from the displaced live content (Accept Source over a
-- conflict), so finalization displaces the right Baseline without the
-- application mutating it before the operation commits.
ALTER TABLE store_operations ADD COLUMN baseline_digest TEXT NOT NULL DEFAULT '';

-- The Baseline-only refresh (decision 05 independent convergence and
-- Keep Store) journals through the same store_operations machinery as
-- imports and replaces: the kind set gains 'baseline', whose finalization
-- moves the staged tree into the Baseline slot instead of touching live
-- content. SQLite cannot alter a CHECK constraint, so the table is rebuilt
-- like migration 006, preserving the AUTOINCREMENT high-water mark and the
-- exact receipt invariants.
CREATE TABLE store_operations_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    skill_id INTEGER REFERENCES skills(id),
    slug TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('import', 'replace', 'baseline')),
    old_digest TEXT NOT NULL DEFAULT '',
    new_digest TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT 'pending' CHECK (phase IN ('pending', 'committed', 'aborted', 'finalized', 'restored')),
    terminal_receipt BLOB,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    baseline TEXT NOT NULL DEFAULT 'advance' CHECK (baseline IN ('advance', 'keep')),
    baseline_digest TEXT NOT NULL DEFAULT '',
    CHECK (
        (phase IN ('pending', 'committed', 'aborted') AND terminal_receipt IS NULL)
        OR (phase IN ('finalized', 'restored') AND terminal_receipt IS NOT NULL
            AND typeof(terminal_receipt) = 'blob' AND length(terminal_receipt) > 0)
    )
);

INSERT INTO sqlite_sequence (name, seq)
SELECT 'store_operations_new',
       MAX(COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'store_operations'), 0),
           COALESCE((SELECT MAX(id) FROM store_operations), 0));

INSERT INTO store_operations_new (id, skill_id, slug, kind, old_digest, new_digest, phase, terminal_receipt, created_at, updated_at, baseline, baseline_digest)
    SELECT id, skill_id, slug, kind, old_digest, new_digest, phase, terminal_receipt, created_at, updated_at, baseline, baseline_digest
    FROM store_operations;

DROP TABLE store_operations;

ALTER TABLE store_operations_new RENAME TO store_operations;
