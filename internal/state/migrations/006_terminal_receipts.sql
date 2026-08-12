-- Durable terminal receipts for Store operations.
--
-- Stage A ended each operation by removing the physical evidence (the
-- operation directory and its install proof) and then deleting the SQLite
-- operation row. When that row deletion failed, recovery saw the old phase
-- with the evidence already gone and could never attribute the state, so it
-- blocked forever. Migration 006 adds a durable terminal receipt: the Store
-- performs the semantic terminal mutation first, the receipt is CAS-bound
-- to the row (phase 'finalized' or 'restored'), the receipt-authorized
-- evidence cleanup runs, and only then is the row deleted by the exact
-- receipt CAS. The CHECK makes the invariant structural: non-terminal
-- phases never carry a receipt and terminal phases always carry one, so a
-- receipt can never be detached from a terminal row or attached to a
-- pending/committed/aborted one.
CREATE TABLE store_operations_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    skill_id INTEGER REFERENCES skills(id),
    slug TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('import', 'replace')),
    old_digest TEXT NOT NULL DEFAULT '',
    new_digest TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT 'pending' CHECK (phase IN ('pending', 'committed', 'aborted', 'finalized', 'restored')),
    terminal_receipt BLOB,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (phase IN ('pending', 'committed', 'aborted') AND terminal_receipt IS NULL)
        OR (phase IN ('finalized', 'restored') AND terminal_receipt IS NOT NULL
            AND typeof(terminal_receipt) = 'blob' AND length(terminal_receipt) > 0)
    )
);

-- Preserve the AUTOINCREMENT high-water mark across the rebuild: the
-- explicit-id copy below moves the sequence only to the copied maximum,
-- and a rebuild from an empty row set would lose the old sequence
-- entirely (deleting rows never resets sqlite_sequence). Operation ids
-- name filesystem staging paths, so a reused id could collide with a
-- leftover operation directory. The row is renamed with the table, so the
-- captured value survives the DROP and the RENAME.
INSERT INTO sqlite_sequence (name, seq)
SELECT 'store_operations_new',
       MAX(COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'store_operations'), 0),
           COALESCE((SELECT MAX(id) FROM store_operations), 0));

INSERT INTO store_operations_new (id, skill_id, slug, kind, old_digest, new_digest, phase, created_at, updated_at)
    SELECT id, skill_id, slug, kind, old_digest, new_digest, phase, created_at, updated_at
    FROM store_operations;

DROP TABLE store_operations;

ALTER TABLE store_operations_new RENAME TO store_operations;
