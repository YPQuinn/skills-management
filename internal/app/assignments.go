package app

import (
	"database/sql"
	"errors"
	"time"

	"skillctl/internal/state"
)

// AssignmentView is one Assignment with its subject's operator-facing
// identity resolved: the Skill for a skill Assignment, the Group for a
// group Assignment.
type AssignmentView struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Skill     *SkillRef `json:"skill,omitempty"`
	Group     *GroupRef `json:"group,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// DesireReason is one contributing Assignment in a desired-set expansion:
// a direct skill Assignment, or a Group Assignment whose Group contains
// the Skill.
type DesireReason struct {
	AssignmentID int64  `json:"assignment_id"`
	Kind         string `json:"kind"`
	GroupID      int64  `json:"group_id,omitempty"`
	GroupName    string `json:"group_name,omitempty"`
}

// DesiredSkill is one Skill in a Target's desired set with every
// Assignment that selects it.
type DesiredSkill struct {
	ID      int64          `json:"id"`
	Slug    string         `json:"slug"`
	Name    string         `json:"name"`
	Reasons []DesireReason `json:"reasons"`
}

// AssignInput is one Assignment request: exactly one of SkillID or
// GroupID, selected by Kind.
type AssignInput struct {
	Kind    string
	SkillID int64
	GroupID int64
}

// ReplaceImpact previews the Groups and Targets affected by replacing one
// Skill: the Groups containing it, and the Targets whose desired set
// includes it directly or through assigned Groups.
type ReplaceImpact struct {
	Groups  []GroupRef     `json:"groups"`
	Targets []TargetImpact `json:"targets"`
}

// TargetImpact is one affected Target with how the Skill reaches it.
type TargetImpact struct {
	TargetRef
	Direct bool       `json:"direct"`
	Groups []GroupRef `json:"groups,omitempty"`
}

// AssignTarget declares that one Skill or Group should be present at a
// Target. Assignment only changes desired state; it never touches a
// Target's filesystem. Re-assigning the same subject is a no-op.
func (a *App) AssignTarget(targetID int64, in AssignInput) (*AssignmentView, error) {
	if _, err := state.GetTargetByID(a.db, targetID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", targetID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", targetID, err)
	}
	as := state.Assignment{TargetID: targetID, CreatedAt: time.Now().UTC()}
	switch in.Kind {
	case "skill":
		if in.SkillID == 0 {
			return nil, Errorf(CodeInvalidArgument, "a Skill Assignment requires a skill_id")
		}
		// Direct Skill Assignments share the Store lock with Skill
		// deletion (Store first) so a concurrent assign cannot insert
		// after the Store tree is gone and before the Skill row is.
		held, err := a.acquireStoreSharedLock()
		if err != nil {
			return nil, err
		}
		defer held.Unlock()
		if _, err := state.GetSkillDetailByID(a.db, in.SkillID); errors.Is(err, sql.ErrNoRows) {
			return nil, Errorf(CodeNotFound, "Skill %d not found", in.SkillID)
		} else if err != nil {
			return nil, Errorf(CodeInternal, "reading Skill %d: %v", in.SkillID, err)
		}
		as.Kind, as.SkillID = "skill", in.SkillID
	case "group":
		if in.GroupID == 0 {
			return nil, Errorf(CodeInvalidArgument, "a Group Assignment requires a group_id")
		}
		if _, err := state.GetGroupByID(a.db, in.GroupID); errors.Is(err, sql.ErrNoRows) {
			return nil, Errorf(CodeNotFound, "Group %d not found", in.GroupID)
		} else if err != nil {
			return nil, Errorf(CodeInternal, "reading Group %d: %v", in.GroupID, err)
		}
		as.Kind, as.GroupID = "group", in.GroupID
	default:
		return nil, Errorf(CodeInvalidArgument, "Assignment kind must be %q or %q", "skill", "group")
	}
	id, _, err := state.InsertAssignment(a.db, as)
	if err != nil {
		return nil, Errorf(CodeInternal, "saving Assignment: %v", err)
	}
	d, err := state.GetAssignmentByID(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Assignment %d: %v", id, err)
	}
	v := assignmentView(*d)
	return &v, nil
}

// UnassignTarget removes one Assignment by subject. Assignment removal
// only changes desired state; it never touches a Target's filesystem.
func (a *App) UnassignTarget(targetID int64, kind string, subjectID int64) (*TargetView, error) {
	if _, err := state.GetTargetByID(a.db, targetID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", targetID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", targetID, err)
	}
	if kind != "skill" && kind != "group" {
		return nil, Errorf(CodeInvalidArgument, "Assignment kind must be %q or %q", "skill", "group")
	}
	if subjectID == 0 {
		return nil, Errorf(CodeInvalidArgument, "unassign requires the Skill or Group subject")
	}
	if err := state.DeleteTargetAssignmentBySubject(a.db, targetID, kind, subjectID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "no %s Assignment %d on Target %d", kind, subjectID, targetID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "removing Assignment: %v", err)
	}
	return a.ShowTarget(targetID)
}

// UnassignTargetByID removes one Assignment by its id. It returns the
// refreshed Target view.
func (a *App) UnassignTargetByID(targetID, assignmentID int64) (*TargetView, error) {
	if _, err := state.GetTargetByID(a.db, targetID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", targetID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", targetID, err)
	}
	d, err := state.GetAssignmentByID(a.db, assignmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Assignment %d not found", assignmentID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Assignment %d: %v", assignmentID, err)
	}
	if d.TargetID != targetID {
		return nil, Errorf(CodeNotFound, "Assignment %d does not belong to Target %d", assignmentID, targetID)
	}
	if err := state.DeleteAssignment(a.db, assignmentID); err != nil {
		return nil, Errorf(CodeInternal, "removing Assignment: %v", err)
	}
	return a.ShowTarget(targetID)
}

// TargetDesiredSet returns one Target's desired Skill set: the union of
// directly assigned Skills and the current members of all assigned Groups,
// deduplicated by Skill id, with every contributing Assignment as a
// reason.
func (a *App) TargetDesiredSet(targetID int64) ([]DesiredSkill, error) {
	if _, err := state.GetTargetByID(a.db, targetID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", targetID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", targetID, err)
	}
	return a.desiredSkills(targetID)
}

// desiredSkills expands the desired set for a Target that is known to
// exist.
func (a *App) desiredSkills(targetID int64) ([]DesiredSkill, error) {
	rows, err := state.ExpandDesiredSet(a.db, targetID)
	if err != nil {
		return nil, Errorf(CodeInternal, "expanding the desired set of Target %d: %v", targetID, err)
	}
	out := make([]DesiredSkill, 0, len(rows))
	for _, r := range rows {
		d := DesiredSkill{
			ID: r.Skill.ID, Slug: r.Skill.Slug, Name: r.Skill.Name,
			Reasons: make([]DesireReason, 0, len(r.Reasons)),
		}
		for _, reason := range r.Reasons {
			d.Reasons = append(d.Reasons, DesireReason{
				AssignmentID: reason.AssignmentID, Kind: reason.Kind,
				GroupID: reason.GroupID, GroupName: reason.GroupName,
			})
		}
		out = append(out, d)
	}
	return out, nil
}

// assignmentView converts one state detail into the app-facing view.
func assignmentView(d state.AssignmentDetail) AssignmentView {
	v := AssignmentView{ID: d.ID, Kind: d.Kind, CreatedAt: d.CreatedAt}
	if d.Kind == "skill" {
		v.Skill = &SkillRef{ID: d.SkillID, Slug: d.SkillSlug, Name: d.SkillName}
	} else {
		v.Group = &GroupRef{ID: d.GroupID, Name: d.GroupName}
	}
	return v
}

// replaceImpact returns the Groups and Targets affected by replacing one
// Skill.
func (a *App) replaceImpact(skillID int64) (*ReplaceImpact, error) {
	imp, err := state.GetSkillImpact(a.db, skillID)
	if err != nil {
		return nil, err
	}
	out := &ReplaceImpact{
		Groups:  make([]GroupRef, 0, len(imp.Groups)),
		Targets: make([]TargetImpact, 0, len(imp.Targets)),
	}
	for _, g := range imp.Groups {
		out.Groups = append(out.Groups, GroupRef{ID: g.ID, Name: g.Name})
	}
	for _, t := range imp.Targets {
		ti := TargetImpact{TargetRef: TargetRef{ID: t.ID, Name: t.Name}, Direct: t.Direct, Groups: []GroupRef{}}
		for _, g := range t.Groups {
			ti.Groups = append(ti.Groups, GroupRef{ID: g.ID, Name: g.Name})
		}
		out.Targets = append(out.Targets, ti)
	}
	return out, nil
}
