package state

import (
	"database/sql"
	"time"

	"skillctl/internal/source"
)

// ListSources returns all Sources ordered by name then id, with Inventory
// entry counts and latest check metadata.
func ListSources(db *sql.DB) ([]source.Summary, error) {
	rows, err := db.Query(`SELECT s.id, s.kind, s.location, s.ref, s.subpath, s.name,
		s.available, s.last_error, s.last_check_started_at, s.last_checked_at,
		s.last_check_result, s.last_successful_check_at, s.last_commit, s.last_inventory_digest,
		(SELECT COUNT(*) FROM source_inventory i WHERE i.source_id = s.id)
		FROM sources s ORDER BY s.name, s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []source.Summary
	for rows.Next() {
		var s source.Summary
		var lastStarted, lastChecked, lastSuccess any
		if err := rows.Scan(&s.ID, &s.Kind, &s.Location, &s.Ref, &s.Subpath, &s.Name,
			&s.Available, &s.LastError, &lastStarted, &lastChecked,
			&s.LastCheckResult, &lastSuccess, &s.LastCommit, &s.LastInventoryDigest, &s.EntryCount); err != nil {
			return nil, err
		}
		if s.LastCheckStartedAt, err = timeFromSQL(lastStarted); err != nil {
			return nil, err
		}
		if s.LastCheckedAt, err = timeFromSQL(lastChecked); err != nil {
			return nil, err
		}
		if s.LastSuccessfulCheckAt, err = timeFromSQL(lastSuccess); err != nil {
			return nil, err
		}
		s.Stale = !s.Available
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetSource returns one Source with its Inventory and issues, or
// sql.ErrNoRows when it does not exist. The row, Inventory, and issues are
// read inside one transaction, so the returned Source is always one
// coherent observation generation: a concurrent ReplaceSourceObservation
// either commits before the read snapshot or after it, and can never mix
// new metadata with old children or vice versa.
func GetSource(db *sql.DB, id int64) (*source.Source, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var s source.Source
	var lastStarted, lastChecked, lastSuccess any
	var createdAt, updatedAt string
	err = tx.QueryRow(`SELECT id, kind, location, ref, subpath, name, created_at, updated_at,
		available, last_error, last_check_started_at, last_checked_at, last_check_result,
		last_successful_check_at, last_commit, last_inventory_digest
		FROM sources WHERE id = ?`, id).
		Scan(&s.ID, &s.Kind, &s.Location, &s.Ref, &s.Subpath, &s.Name,
			&createdAt, &updatedAt, &s.Available, &s.LastError,
			&lastStarted, &lastChecked, &s.LastCheckResult,
			&lastSuccess, &s.LastCommit, &s.LastInventoryDigest)
	if err != nil {
		return nil, err
	}
	if s.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, err
	}
	if s.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return nil, err
	}
	if s.LastCheckStartedAt, err = timeFromSQL(lastStarted); err != nil {
		return nil, err
	}
	if s.LastCheckedAt, err = timeFromSQL(lastChecked); err != nil {
		return nil, err
	}
	if s.LastSuccessfulCheckAt, err = timeFromSQL(lastSuccess); err != nil {
		return nil, err
	}
	entries, err := inventoryTx(tx, id)
	if err != nil {
		return nil, err
	}
	issues, err := issuesTx(tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.Entries = entries
	s.Issues = issues
	return &s, nil
}

func inventoryTx(tx *sql.Tx, id int64) ([]source.Entry, error) {
	rows, err := tx.Query(`SELECT relative_dir, name, description, digest FROM source_inventory WHERE source_id = ? ORDER BY relative_dir`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []source.Entry
	for rows.Next() {
		var e source.Entry
		if err := rows.Scan(&e.RelativeDir, &e.Name, &e.Description, &e.Digest); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func issuesTx(tx *sql.Tx, id int64) ([]source.Issue, error) {
	rows, err := tx.Query(`SELECT relative_dir, reason FROM source_inventory_issues WHERE source_id = ? ORDER BY relative_dir`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []source.Issue
	for rows.Next() {
		var i source.Issue
		if err := rows.Scan(&i.RelativeDir, &i.Reason); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// SourceIDByName resolves one operator-facing Source name to its id, or
// sql.ErrNoRows when no Source carries the name. Names are unique, so the
// resolution is unambiguous.
func SourceIDByName(db *sql.DB, name string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM sources WHERE name = ? LIMIT 1`, name).Scan(&id)
	return id, err
}

// SourceNameExists reports whether any Source carries the name.
func SourceNameExists(db *sql.DB, name string) (bool, error) {
	var one int
	err := db.QueryRow(`SELECT 1 FROM sources WHERE name = ? LIMIT 1`, name).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// SourceIDByTuple returns the id of the Source registered with the same
// locator tuple, or sql.ErrNoRows when none exists.
func SourceIDByTuple(db *sql.DB, loc source.Locator) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM sources WHERE kind = ? AND location = ? AND ref = ? AND subpath = ?`,
		loc.Kind, loc.Location, loc.Ref, loc.Subpath).Scan(&id)
	return id, err
}
