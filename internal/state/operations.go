package state

import (
	"database/sql"
	"fmt"
	"time"

	"skillctl/internal/skillstore"
)

// InsertOperation persists a new Store operation intent and returns it with
// its journal id and pending phase filled in. It must be called before any
// Store filesystem mutation so the journal exists before the live path can
// change.
func InsertOperation(db *sql.DB, op skillstore.Operation) (skillstore.Operation, error) {
	now := time.Now().UTC()
	baseline := op.BaselineMode
	if baseline == "" {
		baseline = skillstore.BaselineAdvance
	}
	res, err := db.Exec(`INSERT INTO store_operations
		(skill_id, slug, kind, old_digest, new_digest, phase, baseline, baseline_digest, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullableID(op.SkillID), op.Slug, op.Kind, op.OldDigest, op.NewDigest,
		skillstore.PhasePending, baseline, op.BaselineDigest, timeToSQL(&now), timeToSQL(&now))
	if err != nil {
		return op, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return op, err
	}
	op.ID = id
	op.Phase = skillstore.PhasePending
	op.BaselineMode = baseline
	return op, nil
}

// ListOpenOperations returns every unfinished operation intent in journal
// order, carrying the terminal receipt bytes of finalized/restored rows.
// Rows are removed when an operation completes, so every row here is open;
// the application resolves each one under the Store exclusive lock before
// any Store write.
func ListOpenOperations(db *sql.DB) ([]skillstore.Operation, error) {
	rows, err := db.Query(`SELECT id, skill_id, slug, kind, old_digest, new_digest, phase, baseline, baseline_digest, terminal_receipt
		FROM store_operations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []skillstore.Operation
	for rows.Next() {
		var op skillstore.Operation
		var skillID sql.NullInt64
		if err := rows.Scan(&op.ID, &skillID, &op.Slug, &op.Kind, &op.OldDigest, &op.NewDigest, &op.Phase, &op.BaselineMode, &op.BaselineDigest, &op.Receipt); err != nil {
			return nil, err
		}
		op.SkillID = skillID.Int64
		out = append(out, op)
	}
	return out, rows.Err()
}

// DeleteOperationIntent removes a pending or aborted operation intent by
// the full-identity phase CAS: the row must still carry the same identity
// and the same phase, so a stale or foreign row can never be cleared by an
// ID-only reference. Terminal/pre-install paths clear non-terminal rows
// only after the Store proved every operation artifact absent.
func DeleteOperationIntent(db *sql.DB, op skillstore.Operation) error {
	if op.Phase != skillstore.PhasePending && op.Phase != skillstore.PhaseAborted {
		return fmt.Errorf("operation %d is not a pending or aborted intent", op.ID)
	}
	res, err := db.Exec(`DELETE FROM store_operations
		WHERE id = ? AND phase = ? AND kind = ? AND slug = ?
			AND old_digest = ? AND new_digest = ? AND skill_id IS ?`,
		op.ID, op.Phase, op.Kind, op.Slug,
		op.OldDigest, op.NewDigest, nullableID(op.SkillID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("operation %d does not match the %s intent", op.ID, op.Phase)
	}
	return nil
}

// MarkOperationAborted durably marks a pre-install refusal (a Stage
// failure or an Install refusal before any live mutation) as aborted, so
// later recovery can safely discard the operation's staging without ever
// touching live content — even when the immediate intent deletion fails.
// The update is a compare-and-set: exactly one pending operation with the
// same identity must match, so a stale or foreign journal row can never be
// aborted.
func MarkOperationAborted(db *sql.DB, op skillstore.Operation) error {
	now := time.Now().UTC()
	res, err := db.Exec(`UPDATE store_operations SET
		phase = ?, updated_at = ?
		WHERE id = ? AND phase = ? AND kind = ? AND slug = ?
			AND old_digest = ? AND new_digest = ? AND skill_id IS ?`,
		skillstore.PhaseAborted, timeToSQL(&now),
		op.ID, skillstore.PhasePending, op.Kind, op.Slug,
		op.OldDigest, op.NewDigest, nullableID(op.SkillID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("operation %d does not match the pending pre-install abort for %q", op.ID, op.Slug)
	}
	return nil
}

// markOperationCommittedTx flips exactly the pending operation that
// describes the filesystem transition being committed. Matching the
// identity, slug, digests, kind, and pre-commit Skill id prevents a stale or
// foreign journal row from authorizing different content.
func markOperationCommittedTx(tx *sql.Tx, expected skillstore.Operation, skillID int64) error {
	now := time.Now().UTC()
	res, err := tx.Exec(`UPDATE store_operations SET
		phase = ?, skill_id = ?, updated_at = ?
		WHERE id = ? AND phase = ? AND kind = ? AND slug = ?
			AND old_digest = ? AND new_digest = ? AND skill_id IS ?`,
		skillstore.PhaseCommitted, skillID, timeToSQL(&now),
		expected.ID, skillstore.PhasePending, expected.Kind, expected.Slug,
		expected.OldDigest, expected.NewDigest, nullableID(expected.SkillID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("operation %d does not match the pending %s transition for %q", expected.ID, expected.Kind, expected.Slug)
	}
	return nil
}

// MarkOperationFinalized durably binds the terminal finalize receipt: the
// CAS flips exactly one committed operation with the same full identity to
// finalized and attaches the receipt, so a stale or foreign journal row can
// never be certified as finalized. The receipt authorizes the receipt-bound
// evidence cleanup and the later exact terminal-row deletion.
func MarkOperationFinalized(db *sql.DB, op skillstore.Operation) error {
	if op.Phase != skillstore.PhaseFinalized || len(op.Receipt) == 0 {
		return fmt.Errorf("operation %d is not a finalized terminal intent with a receipt", op.ID)
	}
	now := time.Now().UTC()
	res, err := db.Exec(`UPDATE store_operations SET
		phase = ?, terminal_receipt = ?, updated_at = ?
		WHERE id = ? AND phase = ? AND kind = ? AND slug = ?
			AND old_digest = ? AND new_digest = ? AND skill_id IS ?
			AND terminal_receipt IS NULL`,
		skillstore.PhaseFinalized, op.Receipt, timeToSQL(&now),
		op.ID, skillstore.PhaseCommitted, op.Kind, op.Slug,
		op.OldDigest, op.NewDigest, nullableID(op.SkillID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("operation %d does not match the committed finalize transition for %q", op.ID, op.Slug)
	}
	return nil
}

// MarkOperationRestored durably binds the terminal restore receipt: the
// CAS flips exactly one pending operation with the same full identity to
// restored and attaches the receipt, so a stale or foreign journal row can
// never be certified as restored.
func MarkOperationRestored(db *sql.DB, op skillstore.Operation) error {
	if op.Phase != skillstore.PhaseRestored || len(op.Receipt) == 0 {
		return fmt.Errorf("operation %d is not a restored terminal intent with a receipt", op.ID)
	}
	now := time.Now().UTC()
	res, err := db.Exec(`UPDATE store_operations SET
		phase = ?, terminal_receipt = ?, updated_at = ?
		WHERE id = ? AND phase = ? AND kind = ? AND slug = ?
			AND old_digest = ? AND new_digest = ? AND skill_id IS ?
			AND terminal_receipt IS NULL`,
		skillstore.PhaseRestored, op.Receipt, timeToSQL(&now),
		op.ID, skillstore.PhasePending, op.Kind, op.Slug,
		op.OldDigest, op.NewDigest, nullableID(op.SkillID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("operation %d does not match the pending restore transition for %q", op.ID, op.Slug)
	}
	return nil
}

// DeleteOperationTerminal removes a terminal operation intent by the exact
// receipt CAS: the row must still carry the same full identity, the same
// terminal phase, and the exact receipt bytes, so a stale, foreign, or
// re-written row can never be cleared by an ID-only reference. Terminal
// rows are deleted only after the receipt-bound evidence cleanup succeeded.
func DeleteOperationTerminal(db *sql.DB, op skillstore.Operation) error {
	if op.Phase != skillstore.PhaseFinalized && op.Phase != skillstore.PhaseRestored {
		return fmt.Errorf("operation %d is not a terminal intent", op.ID)
	}
	if len(op.Receipt) == 0 {
		return fmt.Errorf("operation %d has no terminal receipt", op.ID)
	}
	res, err := db.Exec(`DELETE FROM store_operations
		WHERE id = ? AND phase = ? AND kind = ? AND slug = ?
			AND old_digest = ? AND new_digest = ? AND skill_id IS ?
			AND terminal_receipt = ?`,
		op.ID, op.Phase, op.Kind, op.Slug,
		op.OldDigest, op.NewDigest, nullableID(op.SkillID), op.Receipt)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("operation %d does not match the terminal %s intent with its receipt", op.ID, op.Phase)
	}
	return nil
}

// nullableID maps a zero id to SQL NULL.
func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
