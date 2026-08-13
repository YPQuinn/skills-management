package state

import (
	"database/sql"
	"time"
)

// Assignment is one declaration that a Skill or Group should be present at
// a Target (decision 06). Exactly one of SkillID and GroupID is non-zero,
// selected by Kind.
type Assignment struct {
	ID        int64
	TargetID  int64
	Kind      string
	SkillID   int64
	GroupID   int64
	CreatedAt time.Time
}

// AssignmentDetail is one Assignment with its subject's operator-facing
// identity resolved: the Skill slug and name for a skill Assignment, or the
// Group name for a group Assignment.
type AssignmentDetail struct {
	Assignment
	SkillSlug string
	SkillName string
	GroupName string
}

// InsertAssignment persists one Assignment idempotently: a duplicate
// (same Target, kind, and subject) returns the existing row with created
// set to false. INSERT OR IGNORE makes the idempotency safe under
// concurrent duplicates — the unique indexes reject the second insert
// instead of surfacing a UNIQUE violation — and the follow-up SELECT
// reports the winning row.
func InsertAssignment(db *sql.DB, a Assignment) (id int64, created bool, err error) {
	res, err := db.Exec(`INSERT OR IGNORE INTO assignments (target_id, kind, skill_id, group_id, created_at)
		VALUES (?, ?, ?, ?, ?)`, a.TargetID, a.Kind, nullableID(a.SkillID), nullableID(a.GroupID), timeToSQL(&a.CreatedAt))
	if err != nil {
		return 0, false, err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		id, err = res.LastInsertId()
		return id, true, err
	}
	if a.Kind == "group" {
		err = db.QueryRow(`SELECT id FROM assignments
			WHERE target_id = ? AND kind = 'group' AND group_id = ?`, a.TargetID, a.GroupID).Scan(&id)
	} else {
		err = db.QueryRow(`SELECT id FROM assignments
			WHERE target_id = ? AND kind = 'skill' AND skill_id = ?`, a.TargetID, a.SkillID).Scan(&id)
	}
	return id, false, err
}

// ListTargetAssignmentDetails returns one Target's Assignments with
// resolved subjects, ordered by id.
func ListTargetAssignmentDetails(db *sql.DB, targetID int64) ([]AssignmentDetail, error) {
	rows, err := db.Query(`SELECT a.id, a.target_id, a.kind, COALESCE(a.skill_id, 0), COALESCE(a.group_id, 0), a.created_at,
			COALESCE(s.slug, ''), COALESCE(s.name, ''), COALESCE(g.name, '')
		FROM assignments a
		LEFT JOIN skills s ON a.kind = 'skill' AND s.id = a.skill_id
		LEFT JOIN groups g ON a.kind = 'group' AND g.id = a.group_id
		WHERE a.target_id = ? ORDER BY a.id`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssignmentDetail
	for rows.Next() {
		var d AssignmentDetail
		var createdAt string
		if err := rows.Scan(&d.ID, &d.TargetID, &d.Kind, &d.SkillID, &d.GroupID, &createdAt,
			&d.SkillSlug, &d.SkillName, &d.GroupName); err != nil {
			return nil, err
		}
		if d.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetAssignmentByID returns one Assignment with its resolved subject, or
// sql.ErrNoRows.
func GetAssignmentByID(db *sql.DB, id int64) (*AssignmentDetail, error) {
	row := db.QueryRow(`SELECT a.id, a.target_id, a.kind, COALESCE(a.skill_id, 0), COALESCE(a.group_id, 0), a.created_at,
			COALESCE(s.slug, ''), COALESCE(s.name, ''), COALESCE(g.name, '')
		FROM assignments a
		LEFT JOIN skills s ON a.kind = 'skill' AND s.id = a.skill_id
		LEFT JOIN groups g ON a.kind = 'group' AND g.id = a.group_id
		WHERE a.id = ?`, id)
	var d AssignmentDetail
	var createdAt string
	if err := row.Scan(&d.ID, &d.TargetID, &d.Kind, &d.SkillID, &d.GroupID, &createdAt,
		&d.SkillSlug, &d.SkillName, &d.GroupName); err != nil {
		return nil, err
	}
	var err error
	if d.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, err
	}
	return &d, nil
}

// DeleteAssignment removes one Assignment by id; deleting a row that does
// not exist reports sql.ErrNoRows.
func DeleteAssignment(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM assignments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteTargetAssignmentBySubject removes the Assignment of one Skill or
// Group subject to one Target; removing a non-existent Assignment reports
// sql.ErrNoRows.
func DeleteTargetAssignmentBySubject(db *sql.DB, targetID int64, kind string, subjectID int64) error {
	if kind == "group" {
		res, err := db.Exec(`DELETE FROM assignments
			WHERE target_id = ? AND kind = 'group' AND group_id = ?`, targetID, subjectID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return sql.ErrNoRows
		}
		return nil
	}
	res, err := db.Exec(`DELETE FROM assignments
		WHERE target_id = ? AND kind = 'skill' AND skill_id = ?`, targetID, subjectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
