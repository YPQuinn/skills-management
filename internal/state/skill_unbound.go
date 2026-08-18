package state

import (
	"database/sql"
	"fmt"
)

// InsertUnboundSkill persists one Skill with no Source Binding. Recovery
// uses it when state.db is rebuilt from live Store directories that cannot
// prove an upstream association.
func InsertUnboundSkill(db *sql.DB, s Skill) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	id, err := insertSkillTx(tx, s)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE skills SET sync_status = ? WHERE id = ?`, "unbound", id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// ReserveAutoIncrement raises the AUTOINCREMENT high-water mark of table
// so the next insert is strictly greater than atLeast. Leftover Store
// internal directories are named by old ids; a rebuilt empty database
// must not reuse those ids and accidentally claim the leftover trees.
func ReserveAutoIncrement(db *sql.DB, table string, atLeast int64) error {
	if atLeast < 1 {
		return nil
	}
	var current sql.NullInt64
	err := db.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = ?`, table).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("reading sqlite_sequence for %s: %v", table, err)
	}
	if current.Valid && current.Int64 >= atLeast {
		return nil
	}
	if !current.Valid {
		_, err = db.Exec(`INSERT INTO sqlite_sequence(name, seq) VALUES(?, ?)`, table, atLeast)
	} else {
		_, err = db.Exec(`UPDATE sqlite_sequence SET seq = ? WHERE name = ?`, atLeast, table)
	}
	if err != nil {
		return fmt.Errorf("reserving %s ids past %d: %v", table, atLeast, err)
	}
	return nil
}
