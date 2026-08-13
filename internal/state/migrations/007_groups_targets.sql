-- Groups, Targets, and Assignments (decisions 04, 06, 07): overlapping
-- non-owning Skill sets, fixed physical-path Target identity, and the
-- Assignment relationships whose union is a Target's desired Skill set.
-- The requested adapter, scope, and project root on a Target row are
-- explanatory creation metadata; identity is the unique resolved physical
-- path, so a later adapter-table or environment change never moves an
-- existing Target. Group membership and Assignment changes only alter the
-- desired set; they never touch a Target's filesystem.
CREATE TABLE groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE group_members (
    group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    skill_id INTEGER NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    added_at TEXT NOT NULL,
    PRIMARY KEY (group_id, skill_id)
);

CREATE TABLE targets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    path TEXT NOT NULL UNIQUE,
    adapter TEXT NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('user', 'project', 'custom')),
    project_root TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Referenced Skills and Groups are protected by default: RESTRICT keeps a
-- row deletion from silently dropping Assignments, so the later explicit
-- cleanup flow must remove them first. A Target deletion carries its
-- Assignments with it (CASCADE).
CREATE TABLE assignments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id INTEGER NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('skill', 'group')),
    skill_id INTEGER REFERENCES skills(id) ON DELETE RESTRICT,
    group_id INTEGER REFERENCES groups(id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL,
    CHECK (
        (kind = 'skill' AND skill_id IS NOT NULL AND group_id IS NULL)
        OR (kind = 'group' AND group_id IS NOT NULL AND skill_id IS NULL)
    )
);

-- SQLite UNIQUE indexes treat NULLs as distinct, so each index only ever
-- constrains its own kind (the CHECK guarantees the other subject column
-- is NULL), and duplicates of either kind are rejected.
CREATE UNIQUE INDEX assignments_target_skill ON assignments (target_id, skill_id);
CREATE UNIQUE INDEX assignments_target_group ON assignments (target_id, group_id);
