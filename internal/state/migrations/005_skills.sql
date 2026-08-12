-- Skills, their Source Bindings, and the durable Store operation journal.
-- A Skill is one authoritative Store tree identified by a globally unique
-- slug; its Binding records the upstream Source entry it was accepted from.
-- store_digest is the canonical digest of the current Store tree;
-- baseline_digest is the accepted content anchoring the synchronization
-- three-way comparison (import establishes it; later synchronization
-- actions advance it).
CREATE TABLE skills (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    store_digest TEXT NOT NULL,
    baseline_digest TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE source_bindings (
    skill_id INTEGER PRIMARY KEY REFERENCES skills(id) ON DELETE RESTRICT,
    source_id INTEGER NOT NULL REFERENCES sources(id),
    relative_dir TEXT NOT NULL,
    digest TEXT NOT NULL,
    source_commit TEXT NOT NULL DEFAULT '',
    imported_at TEXT NOT NULL,
    UNIQUE (source_id, relative_dir)
);

-- The durable Store operation intent: the journal that makes live-tree
-- replacement crash-recoverable. The intent is inserted before any
-- filesystem mutation, and its phase flips to 'committed' in the same
-- SQLite transaction that commits the Skill row and Binding; recovery
-- restores pending operations and finalizes committed ones. The id names
-- the operation's staging and recovery directories inside the Store.
CREATE TABLE store_operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    skill_id INTEGER REFERENCES skills(id),
    slug TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('import', 'replace')),
    old_digest TEXT NOT NULL DEFAULT '',
    new_digest TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT 'pending' CHECK (phase IN ('pending', 'committed', 'aborted')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- The single previous snapshot of each Skill (decision 05): at most one row
-- per Skill, committed atomically with the replace that rotated the
-- snapshot. It records the digest, the Source evidence at replace time, the
-- reason, and the time, so a later rollback can validate the physical
-- .skillctl/previous/<skillID> tree against the metadata before using it.
CREATE TABLE skill_snapshots (
    skill_id INTEGER PRIMARY KEY REFERENCES skills(id) ON DELETE CASCADE,
    digest TEXT NOT NULL,
    source_commit TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL,
    created_at TEXT NOT NULL
);
