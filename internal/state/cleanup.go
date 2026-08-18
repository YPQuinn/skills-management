package state

import (
	"database/sql"
	"fmt"
	"time"

	"skillctl/internal/skillstore"
)

// ManagedLinkRef is one ledger row with the Target name and Skill slug
// resolved, for cleanup previews.
type ManagedLinkRef struct {
	TargetID   int64
	TargetName string
	SkillID    int64
	Slug       string
	LinkPath   string
	RawTarget  string
}

// ListManagedLinksBySkill returns every ledger row for one Skill, ordered
// by Target id so multi-Target cleanup can lock in stable order.
func ListManagedLinksBySkill(db *sql.DB, skillID int64) ([]ManagedLinkRef, error) {
	rows, err := db.Query(`SELECT ml.target_id, t.name, ml.skill_id, s.slug, ml.link_path, ml.raw_target
		FROM managed_links ml
		JOIN targets t ON t.id = ml.target_id
		JOIN skills s ON s.id = ml.skill_id
		WHERE ml.skill_id = ? ORDER BY ml.target_id, s.slug`, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedLinkRef
	for rows.Next() {
		var l ManagedLinkRef
		if err := rows.Scan(&l.TargetID, &l.TargetName, &l.SkillID, &l.Slug, &l.LinkPath, &l.RawTarget); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListSkillAssignments returns every direct Skill Assignment of one Skill.
func ListSkillAssignments(db *sql.DB, skillID int64) ([]AssignmentDetail, error) {
	rows, err := db.Query(`SELECT a.id, a.target_id, a.kind, COALESCE(a.skill_id, 0), COALESCE(a.group_id, 0), a.created_at,
			COALESCE(s.slug, ''), COALESCE(s.name, ''), COALESCE(g.name, '')
		FROM assignments a
		LEFT JOIN skills s ON a.kind = 'skill' AND s.id = a.skill_id
		LEFT JOIN groups g ON a.kind = 'group' AND g.id = a.group_id
		WHERE a.kind = 'skill' AND a.skill_id = ? ORDER BY a.id`, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssignmentDetails(rows)
}

func scanAssignmentDetails(rows *sql.Rows) ([]AssignmentDetail, error) {
	var out []AssignmentDetail
	for rows.Next() {
		var d AssignmentDetail
		var createdAt string
		if err := rows.Scan(&d.ID, &d.TargetID, &d.Kind, &d.SkillID, &d.GroupID, &createdAt,
			&d.SkillSlug, &d.SkillName, &d.GroupName); err != nil {
			return nil, err
		}
		var err error
		if d.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteSkillAssignments removes every direct Skill Assignment of one Skill.
func DeleteSkillAssignments(db *sql.DB, skillID int64) error {
	_, err := db.Exec(`DELETE FROM assignments WHERE kind = 'skill' AND skill_id = ?`, skillID)
	return err
}

// DeleteSkillGroupMemberships removes one Skill from every Group.
func DeleteSkillGroupMemberships(db *sql.DB, skillID int64) error {
	_, err := db.Exec(`DELETE FROM group_members WHERE skill_id = ?`, skillID)
	return err
}

// DeleteGroupAssignments removes every Group Assignment of one Group.
func DeleteGroupAssignments(db *sql.DB, groupID int64) error {
	_, err := db.Exec(`DELETE FROM assignments WHERE kind = 'group' AND group_id = ?`, groupID)
	return err
}

// DeleteBinding removes one Skill's Source Binding. Missing is a no-op.
func DeleteBinding(db *sql.DB, skillID int64) error {
	_, err := db.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, skillID)
	return err
}

// UpsertBinding writes one Skill's Source Binding: insert when unbound,
// replace when already bound. The (source, relative_dir) uniqueness is
// left to the schema.
func UpsertBinding(db *sql.DB, b Binding) error {
	_, err := db.Exec(`INSERT INTO source_bindings
		(skill_id, source_id, relative_dir, digest, source_commit, imported_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (skill_id) DO UPDATE SET
			source_id = excluded.source_id,
			relative_dir = excluded.relative_dir,
			digest = excluded.digest,
			source_commit = excluded.source_commit,
			imported_at = excluded.imported_at`,
		b.SkillID, b.SourceID, b.RelativeDir, b.Digest, b.SourceCommit, timeToSQL(&b.ImportedAt))
	return err
}

// ClearSkillBaseline clears the persisted Baseline digest after Detach or
// a conflicting Rebind, so a later check cannot treat leftover comparison
// state as an accepted Source.
func ClearSkillBaseline(db *sql.DB, skillID int64, now time.Time) error {
	return SetSkillBaselineDigest(db, skillID, "", now)
}

// SetSkillBaselineDigest writes one Skill's Baseline digest, used to
// restore the pre-Rebind value when a later step fails.
func SetSkillBaselineDigest(db *sql.DB, skillID int64, digest string, now time.Time) error {
	res, err := db.Exec(`UPDATE skills SET baseline_digest = ?, updated_at = ? WHERE id = ?`,
		digest, timeToSQL(&now), skillID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("updating Baseline of Skill %d: no matching row", skillID)
	}
	return nil
}

// DeleteSkillRow removes one Skill and its Binding after Assignments,
// Managed Links, and observations have been cleared. Open link intents
// must already have been recovered; leftover intents fail the delete
// instead of being dropped.
func DeleteSkillRow(db *sql.DB, skillID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, skillID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM distribution_items WHERE skill_id = ?`, skillID); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM skills WHERE id = ?`, skillID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("deleting Skill %d: no matching row", skillID)
	}
	return tx.Commit()
}

// CommitSkillDelete deletes the Skill row and marks the pending remove
// journal committed in one transaction. The operation's skill_id is
// nulled first so the skills FK does not block the delete; recovery of a
// committed remove uses the operation id and slug, not the Skill row.
func CommitSkillDelete(db *sql.DB, op skillstore.Operation, skillID int64) error {
	if op.Kind != skillstore.KindRemove || skillID == 0 {
		return fmt.Errorf("CommitSkillDelete %q: invalid remove", op.Slug)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := timeToSQL(nowUTC())
	res, err := tx.Exec(`UPDATE store_operations SET
		phase = ?, skill_id = NULL, updated_at = ?
		WHERE id = ? AND phase = ? AND kind = ? AND slug = ?
			AND old_digest = ? AND new_digest = ? AND skill_id IS ?`,
		skillstore.PhaseCommitted, now,
		op.ID, skillstore.PhasePending, op.Kind, op.Slug,
		op.OldDigest, op.NewDigest, nullableID(skillID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("operation %d does not match the pending remove of Skill %d", op.ID, skillID)
	}
	if _, err := tx.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, skillID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM distribution_items WHERE skill_id = ?`, skillID); err != nil {
		return err
	}
	res, err = tx.Exec(`DELETE FROM skills WHERE id = ?`, skillID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("deleting Skill %d: no matching row", skillID)
	}
	return tx.Commit()
}

// DeleteSource removes one Source. Bindings must already be gone; Inventory
// rows cascade.
func DeleteSource(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM sources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteTarget removes one Target registration. Assignments, ledger rows,
// observations, and intents cascade with the Target.
func DeleteTarget(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM targets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteGroup removes one Group. Memberships cascade; Assignments must
// already be gone.
func DeleteGroup(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
