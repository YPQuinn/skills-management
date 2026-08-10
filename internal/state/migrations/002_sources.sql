-- Sources and their whole-replacement Inventory. Observation state
-- (availability, staleness) lives on the Source row; valid entries and
-- validation issues are replaced wholesale on every successful check.
CREATE TABLE sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL CHECK (kind IN ('local', 'git')),
    location TEXT NOT NULL,
    ref TEXT NOT NULL DEFAULT '',
    subpath TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    available INTEGER NOT NULL DEFAULT 1,
    last_error TEXT NOT NULL DEFAULT '',
    last_checked_at TEXT,
    last_successful_check_at TEXT,
    last_commit TEXT NOT NULL DEFAULT '',
    UNIQUE (kind, location, ref, subpath)
);

CREATE TABLE source_inventory (
    source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    relative_dir TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    PRIMARY KEY (source_id, relative_dir)
);

CREATE TABLE source_inventory_issues (
    source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    relative_dir TEXT NOT NULL,
    reason TEXT NOT NULL,
    PRIMARY KEY (source_id, relative_dir)
);
