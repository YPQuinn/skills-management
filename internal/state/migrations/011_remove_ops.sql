-- Skill delete and Baseline-clear journals (cleanup crash windows).
--
-- Kind 'remove' isolates live/baseline/previous, then the same SQLite
-- transaction deletes the Skill row and marks the intent committed.
-- Kind 'baseline' with baseline='clear' isolates the Baseline tree
-- before the Binding/digest/status commit. SQLite cannot alter a CHECK,
-- so the table is rebuilt like 006/008, preserving the AUTOINCREMENT
-- high-water mark and the terminal-receipt invariant.
CREATE TABLE store_operations_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    skill_id INTEGER REFERENCES skills(id),
    slug TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('import', 'replace', 'baseline', 'remove')),
    old_digest TEXT NOT NULL DEFAULT '',
    new_digest TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT 'pending' CHECK (phase IN ('pending', 'committed', 'aborted', 'finalized', 'restored')),
    terminal_receipt BLOB,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    baseline TEXT NOT NULL DEFAULT 'advance' CHECK (baseline IN ('advance', 'keep', 'clear')),
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
