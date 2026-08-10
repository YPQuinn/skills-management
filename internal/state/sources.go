package state

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"skillctl/internal/source"
)

// timeToSQL stores a timestamp as UTC RFC3339 with fractional-second
// precision (RFC3339Nano); nil stays NULL. Fractional precision makes two
// checks in the same second visibly advance.
func timeToSQL(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// timeFromSQL parses a stored timestamp. RFC3339Nano accepts both
// fractional-second values written by current builds and the legacy
// second-precision values written by earlier ones.
func timeFromSQL(v any) (*time.Time, error) {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, fmt.Errorf("stored timestamp %q: %v", s, err)
	}
	return &t, nil
}

// InsertSource persists a newly registered Source with its first Inventory
// replacement in one transaction.
func InsertSource(db *sql.DB, s source.Source) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO sources
		(kind, location, ref, subpath, name, created_at, updated_at,
		 available, last_error, last_check_started_at, last_checked_at,
		 last_check_result, last_successful_check_at, last_commit, last_inventory_digest)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Kind, s.Location, s.Ref, s.Subpath, s.Name,
		timeToSQL(&s.CreatedAt), timeToSQL(&s.UpdatedAt),
		s.Available, s.LastError, timeToSQL(s.LastCheckStartedAt), timeToSQL(s.LastCheckedAt),
		s.LastCheckResult, timeToSQL(s.LastSuccessfulCheckAt), s.LastCommit, s.LastInventoryDigest)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := replaceInventory(tx, id, s.Entries, s.Issues); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// ReplaceSourceObservation persists a successful re-check: status and check
// metadata fields, the whole Inventory replacement, and the new commit and
// aggregate digest in one transaction.
func ReplaceSourceObservation(db *sql.DB, id int64, s source.Source) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE sources SET
		updated_at = ?, available = ?, last_error = ?, last_check_started_at = ?,
		last_checked_at = ?, last_check_result = ?, last_successful_check_at = ?,
		last_commit = ?, last_inventory_digest = ?
		WHERE id = ?`,
		timeToSQL(&s.UpdatedAt), s.Available, s.LastError, timeToSQL(s.LastCheckStartedAt),
		timeToSQL(s.LastCheckedAt), s.LastCheckResult, timeToSQL(s.LastSuccessfulCheckAt),
		s.LastCommit, s.LastInventoryDigest, id); err != nil {
		return err
	}
	if err := replaceInventory(tx, id, s.Entries, s.Issues); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateSourceStatus persists a failed re-check: the previous Inventory,
// commit, and digest are retained; only availability, the observation error,
// and the latest check start/completion/result are updated.
func UpdateSourceStatus(db *sql.DB, id int64, s source.Source) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE sources SET
		updated_at = ?, available = ?, last_error = ?, last_check_started_at = ?,
		last_checked_at = ?, last_check_result = ?
		WHERE id = ?`,
		timeToSQL(&s.UpdatedAt), s.Available, s.LastError, timeToSQL(s.LastCheckStartedAt),
		timeToSQL(s.LastCheckedAt), s.LastCheckResult, id); err != nil {
		return err
	}
	return tx.Commit()
}

// replaceInventory replaces the whole Inventory (valid entries with their
// tree digests, plus issues) for one Source.
func replaceInventory(tx *sql.Tx, id int64, entries []source.Entry, issues []source.Issue) error {
	if _, err := tx.Exec(`DELETE FROM source_inventory WHERE source_id = ?`, id); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := tx.Exec(`INSERT INTO source_inventory (source_id, relative_dir, name, description, digest) VALUES (?, ?, ?, ?, ?)`,
			id, e.RelativeDir, e.Name, e.Description, e.Digest); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM source_inventory_issues WHERE source_id = ?`, id); err != nil {
		return err
	}
	for _, i := range issues {
		if _, err := tx.Exec(`INSERT INTO source_inventory_issues (source_id, relative_dir, reason) VALUES (?, ?, ?)`,
			id, i.RelativeDir, i.Reason); err != nil {
			return err
		}
	}
	return nil
}

// IsUniqueViolation reports whether err is a SQLite UNIQUE constraint
// failure, which the application maps to a conflict error.
func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
