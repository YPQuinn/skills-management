// Package state owns desired-set expansion: the union of directly
// assigned Skills and the current members of all assigned Groups for one
// Target (decision 06).
package state

import (
	"database/sql"
	"sort"
	"time"
)

// DesireReason is one contributing Assignment in a desired-set expansion.
type DesireReason struct {
	AssignmentID int64
	Kind         string
	GroupID      int64
	GroupName    string
}

// DesiredSkill is one Skill in a Target's desired set together with every
// Assignment that selects it.
type DesiredSkill struct {
	Skill
	Reasons []DesireReason
}

// ExpandDesiredSet computes one Target's desired Skill set: the union of
// directly assigned Skills and the current members of all assigned Groups,
// deduplicated by Skill id (decision 06). Every contributing Assignment is
// kept as a reason even when several Assignments select the same Skill.
func ExpandDesiredSet(db *sql.DB, targetID int64) ([]DesiredSkill, error) {
	set := map[int64]*DesiredSkill{}
	add := func(s Skill, r DesireReason) {
		if d, ok := set[s.ID]; ok {
			d.Reasons = append(d.Reasons, r)
			return
		}
		set[s.ID] = &DesiredSkill{Skill: s, Reasons: []DesireReason{r}}
	}

	rows, err := db.Query(`SELECT a.id, s.id, s.slug, s.name, s.description, s.store_digest, s.baseline_digest, s.created_at, s.updated_at
		FROM assignments a JOIN skills s ON s.id = a.skill_id
		WHERE a.target_id = ? AND a.kind = 'skill'`, targetID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s Skill
		var assignmentID int64
		var createdAt, updatedAt string
		if err := rows.Scan(&assignmentID, &s.ID, &s.Slug, &s.Name, &s.Description, &s.StoreDigest, &s.BaselineDigest,
			&createdAt, &updatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if s.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			rows.Close()
			return nil, err
		}
		if s.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		add(s, DesireReason{AssignmentID: assignmentID, Kind: "skill"})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	groupRows, err := db.Query(`SELECT a.id, g.id, g.name
		FROM assignments a JOIN groups g ON g.id = a.group_id
		WHERE a.target_id = ? AND a.kind = 'group'`, targetID)
	if err != nil {
		return nil, err
	}
	type groupAssignment struct {
		assignmentID int64
		groupID      int64
		groupName    string
	}
	var groupAssignments []groupAssignment
	for groupRows.Next() {
		var ga groupAssignment
		if err := groupRows.Scan(&ga.assignmentID, &ga.groupID, &ga.groupName); err != nil {
			groupRows.Close()
			return nil, err
		}
		groupAssignments = append(groupAssignments, ga)
	}
	if err := groupRows.Err(); err != nil {
		groupRows.Close()
		return nil, err
	}
	groupRows.Close()

	for _, ga := range groupAssignments {
		memberRows, err := db.Query(`SELECT s.id, s.slug, s.name, s.description, s.store_digest, s.baseline_digest, s.created_at, s.updated_at
			FROM group_members gm JOIN skills s ON s.id = gm.skill_id
			WHERE gm.group_id = ?`, ga.groupID)
		if err != nil {
			return nil, err
		}
		for memberRows.Next() {
			var s Skill
			var createdAt, updatedAt string
			if err := memberRows.Scan(&s.ID, &s.Slug, &s.Name, &s.Description, &s.StoreDigest, &s.BaselineDigest,
				&createdAt, &updatedAt); err != nil {
				memberRows.Close()
				return nil, err
			}
			if s.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
				memberRows.Close()
				return nil, err
			}
			if s.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
				memberRows.Close()
				return nil, err
			}
			add(s, DesireReason{AssignmentID: ga.assignmentID, Kind: "group", GroupID: ga.groupID, GroupName: ga.groupName})
		}
		if err := memberRows.Err(); err != nil {
			memberRows.Close()
			return nil, err
		}
		memberRows.Close()
	}

	out := make([]DesiredSkill, 0, len(set))
	for _, d := range set {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// TargetImpact describes how one Skill reaches a Target: directly assigned,
// or through assigned Groups that contain it.
type TargetImpact struct {
	ID     int64
	Name   string
	Direct bool
	Groups []GroupRef
}

// SkillImpact is the set of Groups and Targets affected by one Skill: the
// Groups containing it and the Targets whose desired set includes it.
type SkillImpact struct {
	Groups  []GroupRef
	Targets []TargetImpact
}

// GetSkillImpact returns the Groups containing one Skill and the Targets
// whose desired set includes it, for the replacement preview.
func GetSkillImpact(db *sql.DB, skillID int64) (*SkillImpact, error) {
	imp := &SkillImpact{Groups: []GroupRef{}, Targets: []TargetImpact{}}

	rows, err := db.Query(`SELECT g.id, g.name
		FROM group_members gm JOIN groups g ON g.id = gm.group_id
		WHERE gm.skill_id = ? ORDER BY g.name, g.id`, skillID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g GroupRef
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			rows.Close()
			return nil, err
		}
		imp.Groups = append(imp.Groups, g)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	byTarget := map[int64]*TargetImpact{}
	rows, err = db.Query(`SELECT t.id, t.name, 1
		FROM assignments a JOIN targets t ON t.id = a.target_id
		WHERE a.kind = 'skill' AND a.skill_id = ?`, skillID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t TargetImpact
		if err := rows.Scan(&t.ID, &t.Name, &t.Direct); err != nil {
			rows.Close()
			return nil, err
		}
		t.Groups = []GroupRef{}
		byTarget[t.ID] = &t
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = db.Query(`SELECT t.id, t.name, g.id, g.name
		FROM assignments a
		JOIN targets t ON t.id = a.target_id
		JOIN groups g ON g.id = a.group_id
		JOIN group_members gm ON gm.group_id = g.id AND gm.skill_id = ?
		WHERE a.kind = 'group'`, skillID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var tID, gID int64
		var tName, gName string
		if err := rows.Scan(&tID, &tName, &gID, &gName); err != nil {
			rows.Close()
			return nil, err
		}
		ti, ok := byTarget[tID]
		if !ok {
			ti = &TargetImpact{ID: tID, Name: tName, Groups: []GroupRef{}}
			byTarget[tID] = ti
		}
		ti.Groups = append(ti.Groups, GroupRef{ID: gID, Name: gName})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for _, ti := range byTarget {
		imp.Targets = append(imp.Targets, *ti)
	}
	sort.Slice(imp.Targets, func(i, j int) bool { return imp.Targets[i].Name < imp.Targets[j].Name })
	return imp, nil
}
