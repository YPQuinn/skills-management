package state

import (
	"database/sql"
	"fmt"
	"time"

	"skillctl/internal/skillstore"
)

// SyncOutcome is the persisted latest synchronization outcome of one Skill
// (decision 05): the action, result, start and completion times, before and
// after content digests, the observed Source revision, and the failure or
// block detail.
type SyncOutcome struct {
	Action       string
	Result       string
	StartedAt    time.Time
	CompletedAt  time.Time
	BeforeDigest string
	AfterDigest  string
	Revision     string
	Error        string
}

// UpdateSyncStatus persists one Skill's evaluated Sync Status relationship:
// the status, its staleness, and the evaluation time.
func UpdateSyncStatus(db *sql.DB, skillID int64, status string, stale bool, checkedAt time.Time) error {
	res, err := db.Exec(`UPDATE skills SET
		sync_status = ?, sync_stale = ?, sync_checked_at = ?, updated_at = ?
		WHERE id = ?`,
		status, boolInt(stale), timeToSQL(&checkedAt), timeToSQL(&checkedAt), skillID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("updating sync status of Skill %d: no matching row", skillID)
	}
	return nil
}

// UpdateSyncOutcome persists the latest synchronization outcome of one
// Skill.
func UpdateSyncOutcome(db *sql.DB, skillID int64, o SyncOutcome) error {
	res, err := db.Exec(`UPDATE skills SET
		last_sync_action = ?, last_sync_result = ?,
		last_sync_started_at = ?, last_sync_completed_at = ?,
		last_sync_before_digest = ?, last_sync_after_digest = ?,
		last_sync_revision = ?, last_sync_error = ?
		WHERE id = ?`,
		o.Action, o.Result,
		timeToSQL(&o.StartedAt), timeToSQL(&o.CompletedAt),
		o.BeforeDigest, o.AfterDigest, o.Revision, o.Error, skillID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("updating sync outcome of Skill %d: no matching row", skillID)
	}
	return nil
}

// ListSkillsBySource returns every Skill bound to one Source, ordered by
// slug, each with its Binding and Source name.
func ListSkillsBySource(db *sql.DB, sourceID int64) ([]SkillDetail, error) {
	rows, err := db.Query(`SELECT `+skillDetailSelect+` `+skillDetailFrom+`
		WHERE b.source_id = ? ORDER BY s.slug, s.id`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SkillDetail
	for rows.Next() {
		d, err := scanSkillDetail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// CommitRollback persists a committed rollback atomically: the Store digest
// becomes the snapshot digest while the Baseline and the Source Binding are
// deliberately untouched, the pre-rollback live digest becomes the new
// previous snapshot with reason 'rollback', and the operation's phase
// transitions in the same transaction. The operation must be the pending
// keep-baseline replace intent describing exactly the persisted transition.
// The rollback never depends on the Binding: an unbound Skill with a
// previous snapshot still rolls back, and the rotated snapshot records the
// Binding's source commit as its Source evidence — empty when no Binding
// exists.
func CommitRollback(db *sql.DB, s Skill, opID int64) error {
	if s.StoreDigest == "" {
		return fmt.Errorf("CommitRollback %q: empty Store digest", s.Slug)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldStoreDigest string
	var sourceCommit sql.NullString
	if err := tx.QueryRow(`SELECT s.store_digest, b.source_commit
		FROM skills s LEFT JOIN source_bindings b ON b.skill_id = s.id
		WHERE s.id = ? AND s.slug = ?`, s.ID, s.Slug).Scan(&oldStoreDigest, &sourceCommit); err != nil {
		return fmt.Errorf("reading Skill %d: %v", s.ID, err)
	}
	snapshotSourceCommit := ""
	if sourceCommit.Valid {
		snapshotSourceCommit = sourceCommit.String
	}
	var opOldDigest, opNewDigest, opBaseline string
	if err := tx.QueryRow(`SELECT old_digest, new_digest, baseline FROM store_operations
		WHERE id = ? AND phase = ? AND kind = ?`, opID, skillstore.PhasePending, skillstore.KindReplace).
		Scan(&opOldDigest, &opNewDigest, &opBaseline); err != nil {
		return fmt.Errorf("reading pending rollback operation %d: %v", opID, err)
	}
	if opOldDigest != oldStoreDigest {
		return fmt.Errorf("rolling back Skill %d (%s): operation old digest %q does not match the persisted Store digest %q",
			s.ID, s.Slug, opOldDigest, oldStoreDigest)
	}
	if opNewDigest != s.StoreDigest {
		return fmt.Errorf("rolling back Skill %d (%s): operation new digest %q does not match the snapshot digest %q",
			s.ID, s.Slug, opNewDigest, s.StoreDigest)
	}
	if opBaseline != skillstore.BaselineKeep {
		return fmt.Errorf("rolling back Skill %d (%s): operation %d does not retain the Baseline", s.ID, s.Slug, opID)
	}
	res, err := tx.Exec(`UPDATE skills SET store_digest = ?, updated_at = ?
		WHERE id = ? AND slug = ? AND store_digest = ?`,
		s.StoreDigest, timeToSQL(&s.UpdatedAt), s.ID, s.Slug, oldStoreDigest)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("rolling back Skill %d (%s): no matching row or slug changed", s.ID, s.Slug)
	}
	if err := upsertSnapshotTx(tx, s.ID, opID, snapshotSourceCommit, "rollback"); err != nil {
		return err
	}
	expected := skillstore.Operation{
		ID: opID, SkillID: s.ID, Slug: s.Slug, Kind: skillstore.KindReplace,
		OldDigest: oldStoreDigest, NewDigest: s.StoreDigest,
	}
	if err := markOperationCommittedTx(tx, expected, s.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// CommitBaselineAdvance persists a committed Baseline-only refresh
// atomically: the Baseline digest advances by compare-and-set on the
// recorded pre-refresh digest, the Binding records the accepted Source
// content and revision, and the operation's phase transitions in the same
// transaction. The operation must be the pending baseline intent describing
// exactly the persisted transition.
func CommitBaselineAdvance(db *sql.DB, op skillstore.Operation, skillID int64, oldBaseline, newBaseline, sourceCommit string) error {
	if newBaseline == "" || op.Kind != skillstore.KindBaseline {
		return fmt.Errorf("CommitBaselineAdvance %q: invalid baseline refresh", op.Slug)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE skills SET baseline_digest = ?, updated_at = ?
		WHERE id = ? AND baseline_digest = ?`,
		newBaseline, timeToSQL(nowUTC()), skillID, oldBaseline)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("advancing the Baseline of Skill %d: no matching row or the Baseline changed", skillID)
	}
	res, err = tx.Exec(`UPDATE source_bindings SET digest = ?, source_commit = ?
		WHERE skill_id = ?`, newBaseline, sourceCommit, skillID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("recording the accepted Source of Skill %d: Binding is missing", skillID)
	}
	expected := skillstore.Operation{
		ID: op.ID, SkillID: skillID, Slug: op.Slug, Kind: skillstore.KindBaseline,
		OldDigest: oldBaseline, NewDigest: newBaseline,
	}
	if err := markOperationCommittedTx(tx, expected, skillID); err != nil {
		return err
	}
	return tx.Commit()
}

// CommitBaselineClear persists a Baseline-clear journal commit: the
// Baseline digest is cleared by compare-and-set, the Sync Status is
// written, and the operation is marked committed. When b is non-nil the
// Binding is upserted (conflicting Rebind); when b is nil the Binding is
// deleted (Detach). The Baseline tree must already be isolated.
func CommitBaselineClear(db *sql.DB, op skillstore.Operation, skillID int64, oldBaseline string, b *Binding, status string) error {
	if op.Kind != skillstore.KindBaseline || op.BaselineMode != skillstore.BaselineClear {
		return fmt.Errorf("CommitBaselineClear %q: invalid Baseline clear", op.Slug)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := nowUTC()
	if b != nil {
		if _, err := tx.Exec(`INSERT INTO source_bindings
			(skill_id, source_id, relative_dir, digest, source_commit, imported_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (skill_id) DO UPDATE SET
				source_id = excluded.source_id,
				relative_dir = excluded.relative_dir,
				digest = excluded.digest,
				source_commit = excluded.source_commit,
				imported_at = excluded.imported_at`,
			b.SkillID, b.SourceID, b.RelativeDir, b.Digest, b.SourceCommit, timeToSQL(&b.ImportedAt)); err != nil {
			return err
		}
	} else if _, err := tx.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, skillID); err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE skills SET baseline_digest = '', sync_status = ?, sync_stale = 0,
		sync_checked_at = ?, updated_at = ? WHERE id = ? AND baseline_digest = ?`,
		status, timeToSQL(now), timeToSQL(now), skillID, oldBaseline)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("clearing the Baseline of Skill %d: no matching row or the Baseline changed", skillID)
	}
	expected := skillstore.Operation{
		ID: op.ID, SkillID: skillID, Slug: op.Slug, Kind: skillstore.KindBaseline,
		OldDigest: op.OldDigest, NewDigest: op.NewDigest,
	}
	if err := markOperationCommittedTx(tx, expected, skillID); err != nil {
		return err
	}
	return tx.Commit()
}

// CommitRepairImport persists an explicit Accept Source repair of a missing
// Store tree: the existing Skill row and Binding are updated to the
// imported identity (Store, Baseline, and Binding carry the new digest) and
// the operation's phase transitions in one transaction. The Baseline
// advance is compare-and-set on the recorded pre-repair Baseline digest, so
// a concurrent acceptance is never silently overwritten; the physical
// Baseline swap runs in the same journal's finalization. The existing
// previous snapshot is deliberately untouched: there was no live content to
// snapshot.
func CommitRepairImport(db *sql.DB, s Skill, b Binding, opID int64, oldBaseline string) error {
	if s.StoreDigest == "" || s.StoreDigest != b.Digest || s.StoreDigest != s.BaselineDigest {
		return fmt.Errorf("CommitRepairImport %q: impossible accepted identity: store %q, binding %q",
			s.Slug, s.StoreDigest, b.Digest)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldStoreDigest string
	if err := tx.QueryRow(`SELECT store_digest FROM skills WHERE id = ? AND slug = ?`,
		s.ID, s.Slug).Scan(&oldStoreDigest); err != nil {
		return fmt.Errorf("reading Skill %d: %v", s.ID, err)
	}
	res, err := tx.Exec(`UPDATE skills SET
		name = ?, description = ?, store_digest = ?, baseline_digest = ?, updated_at = ?
		WHERE id = ? AND slug = ? AND store_digest = ? AND baseline_digest = ?`,
		s.Name, s.Description, s.StoreDigest, s.BaselineDigest,
		timeToSQL(&s.UpdatedAt), s.ID, s.Slug, oldStoreDigest, oldBaseline)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("repairing Skill %d (%s): no matching row, slug, or Baseline changed", s.ID, s.Slug)
	}
	res, err = tx.Exec(`UPDATE source_bindings SET
		digest = ?, source_commit = ?, imported_at = ?
		WHERE skill_id = ?`,
		b.Digest, b.SourceCommit, timeToSQL(&b.ImportedAt), s.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("repairing Skill %d: Source Binding is missing", s.ID)
	}
	expected := skillstore.Operation{
		ID: opID, Slug: s.Slug, Kind: skillstore.KindImport, NewDigest: s.StoreDigest,
	}
	if err := markOperationCommittedTx(tx, expected, s.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nowUTC() *time.Time {
	now := time.Now().UTC()
	return &now
}
