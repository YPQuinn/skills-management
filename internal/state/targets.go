package state

import (
	"database/sql"
	"time"
)

// Target is one registered Target: its fixed physical path identity plus
// the explanatory creation metadata (requested adapter, scope, and project
// root, decision 04). The path is unique; identity never moves because a
// current working directory, environment, or adapter table changes.
type Target struct {
	ID          int64
	Name        string
	Path        string
	Adapter     string
	Scope       string
	ProjectRoot string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Distribution observation and outcome metadata (decision 06).
	LastInspectedAt    *time.Time
	LastInspectedStale bool
	LastInspectError   string
	LastDistResult     string
	LastDistStartedAt  *time.Time
	LastDistCompleted  *time.Time
	LastDistError      string
}

const targetSelect = `id, name, path, adapter, scope, project_root, created_at, updated_at,
	last_inspected_at, last_inspected_stale, last_inspected_error,
	last_distribute_result, last_distribute_started_at, last_distribute_completed_at, last_distribute_error`

// targetSelectAliased is targetSelect qualified for JOIN queries.
const targetSelectAliased = `t.id, t.name, t.path, t.adapter, t.scope, t.project_root, t.created_at, t.updated_at,
	t.last_inspected_at, t.last_inspected_stale, t.last_inspected_error,
	t.last_distribute_result, t.last_distribute_started_at, t.last_distribute_completed_at, t.last_distribute_error`

func scanTarget(row scanner) (*Target, error) {
	var t Target
	var createdAt, updatedAt string
	var lastInspected, lastDistStarted, lastDistCompleted any
	if err := row.Scan(&t.ID, &t.Name, &t.Path, &t.Adapter, &t.Scope, &t.ProjectRoot,
		&createdAt, &updatedAt,
		&lastInspected, &t.LastInspectedStale, &t.LastInspectError,
		&t.LastDistResult, &lastDistStarted, &lastDistCompleted, &t.LastDistError); err != nil {
		return nil, err
	}
	var err error
	if t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, err
	}
	if t.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return nil, err
	}
	if t.LastInspectedAt, err = timeFromSQL(lastInspected); err != nil {
		return nil, err
	}
	if t.LastDistStartedAt, err = timeFromSQL(lastDistStarted); err != nil {
		return nil, err
	}
	if t.LastDistCompleted, err = timeFromSQL(lastDistCompleted); err != nil {
		return nil, err
	}
	return &t, nil
}

// InsertTarget persists one newly registered Target and returns its id.
func InsertTarget(db *sql.DB, t Target) (int64, error) {
	res, err := db.Exec(`INSERT INTO targets
		(name, path, adapter, scope, project_root, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Path, t.Adapter, t.Scope, t.ProjectRoot,
		timeToSQL(&t.CreatedAt), timeToSQL(&t.UpdatedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListTargets returns all registered Targets ordered by name.
func ListTargets(db *sql.DB) ([]Target, error) {
	rows, err := db.Query(`SELECT ` + targetSelect + ` FROM targets ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetTargetByID returns one Target by numeric id, or sql.ErrNoRows.
func GetTargetByID(db *sql.DB, id int64) (*Target, error) {
	return scanTarget(db.QueryRow(`SELECT `+targetSelect+` FROM targets WHERE id = ?`, id))
}

// GetTargetByPath returns the Target whose fixed physical path is path, or
// sql.ErrNoRows. The path is unique, so at most one Target can match.
func GetTargetByPath(db *sql.DB, path string) (*Target, error) {
	return scanTarget(db.QueryRow(`SELECT `+targetSelect+` FROM targets WHERE path = ?`, path))
}

// TargetIDByName resolves one operator-facing Target name to its id, or
// sql.ErrNoRows.
func TargetIDByName(db *sql.DB, name string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM targets WHERE name = ?`, name).Scan(&id)
	return id, err
}
