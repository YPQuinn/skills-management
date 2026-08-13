package state

import (
	"database/sql"
	"time"
)

// Group is one named, non-owning collection of Skills (decision 06). The
// name is operator-facing and unique; membership is recorded in
// group_members and never implies ownership of a Skill.
type Group struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GroupSummary is one Group list row with its membership count.
type GroupSummary struct {
	Group
	MemberCount int
}

// GroupRef is the minimal identity of one Group for relationship views.
type GroupRef struct {
	ID   int64
	Name string
}

const groupSelect = `id, name, created_at, updated_at`

func scanGroup(row scanner) (*Group, error) {
	var g Group
	var createdAt, updatedAt string
	if err := row.Scan(&g.ID, &g.Name, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var err error
	if g.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, err
	}
	if g.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return nil, err
	}
	return &g, nil
}

// InsertGroup persists one new Group and returns its id.
func InsertGroup(db *sql.DB, g Group) (int64, error) {
	res, err := db.Exec(`INSERT INTO groups (name, created_at, updated_at) VALUES (?, ?, ?)`,
		g.Name, timeToSQL(&g.CreatedAt), timeToSQL(&g.UpdatedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListGroupSummaries returns all Groups in name order with their member
// counts.
func ListGroupSummaries(db *sql.DB) ([]GroupSummary, error) {
	rows, err := db.Query(`SELECT g.id, g.name, g.created_at, g.updated_at, COUNT(gm.skill_id)
		FROM groups g LEFT JOIN group_members gm ON gm.group_id = g.id
		GROUP BY g.id ORDER BY g.name, g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GroupSummary
	for rows.Next() {
		var s GroupSummary
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.Name, &createdAt, &updatedAt, &s.MemberCount); err != nil {
			return nil, err
		}
		var err error
		if s.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, err
		}
		if s.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetGroupByID returns one Group by numeric id, or sql.ErrNoRows.
func GetGroupByID(db *sql.DB, id int64) (*Group, error) {
	return scanGroup(db.QueryRow(`SELECT `+groupSelect+` FROM groups WHERE id = ?`, id))
}

// GetGroupByName returns one Group by unique name, or sql.ErrNoRows.
func GetGroupByName(db *sql.DB, name string) (*Group, error) {
	return scanGroup(db.QueryRow(`SELECT `+groupSelect+` FROM groups WHERE name = ?`, name))
}

// AddGroupMembers makes every given Skill a member of the Group. Adding a
// Skill that is already a member is a no-op; the Skill rows themselves are
// expected to exist (the app resolves them first).
func AddGroupMembers(db *sql.DB, groupID int64, skillIDs []int64, addedAt time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range skillIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO group_members (group_id, skill_id, added_at) VALUES (?, ?, ?)`,
			groupID, id, timeToSQL(&addedAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveGroupMembers removes the given Skills from the Group. Removing a
// Skill that is not a member is a no-op.
func RemoveGroupMembers(db *sql.DB, groupID int64, skillIDs []int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range skillIDs {
		if _, err := tx.Exec(`DELETE FROM group_members WHERE group_id = ? AND skill_id = ?`, groupID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListGroupMemberSkills returns the Skills currently in the Group in slug
// order.
func ListGroupMemberSkills(db *sql.DB, groupID int64) ([]Skill, error) {
	rows, err := db.Query(`SELECT s.id, s.slug, s.name, s.description, s.store_digest, s.baseline_digest, s.created_at, s.updated_at
		FROM group_members gm JOIN skills s ON s.id = gm.skill_id
		WHERE gm.group_id = ? ORDER BY s.slug, s.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Skill
	for rows.Next() {
		var s Skill
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.Slug, &s.Name, &s.Description, &s.StoreDigest, &s.BaselineDigest,
			&createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var err error
		if s.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, err
		}
		if s.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListGroupTargets returns the Targets that assign this Group, in name
// order.
func ListGroupTargets(db *sql.DB, groupID int64) ([]TargetRef, error) {
	rows, err := db.Query(`SELECT DISTINCT t.id, t.name
		FROM assignments a JOIN targets t ON t.id = a.target_id
		WHERE a.kind = 'group' AND a.group_id = ?
		ORDER BY t.name, t.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TargetRef
	for rows.Next() {
		var r TargetRef
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
