-- Distribution state (decision 06): Managed Link ownership, durable link
-- intents, the per-Target-Skill Distribution Status observation, and the
-- per-Target inspection and reconciliation outcomes.
--
-- A Managed Link is provably owned only while its row exists, the path is
-- still a symlink, and its raw target still matches the recorded value.
-- Ownership is never reconstructed from path or naming alone. Link intents
-- are recorded before each link mutation; recovery resolves unfinished
-- intents deterministically (decision 06). Group membership and Assignment
-- changes never touch this state; only explicit Distribution does.
CREATE TABLE managed_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id INTEGER NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    skill_id INTEGER NOT NULL REFERENCES skills(id) ON DELETE RESTRICT,
    link_path TEXT NOT NULL,
    raw_target TEXT NOT NULL,
    established_at TEXT NOT NULL,
    UNIQUE (target_id, skill_id)
);

-- Durable link intents: the action, the exact link path, the expected raw
-- link target, and the filesystem precondition (for removes: the entry must
-- still match its Managed Link record; for creates: the path must accept a
-- no-overwrite link). The row is inserted before the filesystem mutation
-- and deleted together with the final state transition; recovery converges
-- from whatever persisted.
CREATE TABLE link_intents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id INTEGER NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    skill_id INTEGER NOT NULL REFERENCES skills(id) ON DELETE RESTRICT,
    action TEXT NOT NULL CHECK (action IN ('create', 'remove')),
    link_path TEXT NOT NULL,
    raw_target TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- Distribution Status: the desired and last-observed presence of one Skill
-- at one Target, together with observation freshness and the latest
-- reconciliation outcome. Observation details (node kind, raw and resolved
-- link targets, adoption eligibility) travel with the observation so an
-- ordinary listing can return the complete stored observation.
CREATE TABLE distribution_items (
    target_id INTEGER NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    skill_id INTEGER NOT NULL REFERENCES skills(id) ON DELETE RESTRICT,
    desired TEXT NOT NULL CHECK (desired IN ('present', 'absent')),
    observed TEXT NOT NULL CHECK (observed IN ('linked', 'missing', 'conflict', 'broken_link')),
    managed INTEGER NOT NULL DEFAULT 0,
    adoptable INTEGER NOT NULL DEFAULT 0,
    node_kind TEXT NOT NULL DEFAULT '',
    raw_target TEXT NOT NULL DEFAULT '',
    resolved_target TEXT NOT NULL DEFAULT '',
    stale INTEGER NOT NULL DEFAULT 0,
    inspected_at TEXT,
    last_result TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (target_id, skill_id)
);

ALTER TABLE targets ADD COLUMN last_inspected_at TEXT;
ALTER TABLE targets ADD COLUMN last_inspected_stale INTEGER NOT NULL DEFAULT 0;
ALTER TABLE targets ADD COLUMN last_inspected_error TEXT NOT NULL DEFAULT '';
ALTER TABLE targets ADD COLUMN last_distribute_result TEXT NOT NULL DEFAULT '';
ALTER TABLE targets ADD COLUMN last_distribute_started_at TEXT;
ALTER TABLE targets ADD COLUMN last_distribute_completed_at TEXT;
ALTER TABLE targets ADD COLUMN last_distribute_error TEXT NOT NULL DEFAULT '';
