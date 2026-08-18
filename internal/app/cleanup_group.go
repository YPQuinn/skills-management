package app

import (
	"context"
	"database/sql"
	"errors"

	"skillctl/internal/state"
)

// PreviewDeleteGroup reports Targets that assign the Group.
func (a *App) PreviewDeleteGroup(id int64) (*GroupDeletePreview, error) {
	g, err := state.GetGroupByID(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Group %d not found", id)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Group %d: %v", id, err)
	}
	targets, err := state.ListGroupTargets(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Targets of Group %d: %v", id, err)
	}
	out := &GroupDeletePreview{
		Group:   GroupRef{ID: g.ID, Name: g.Name},
		Targets: []TargetRef{},
	}
	for _, t := range targets {
		out.Targets = append(out.Targets, TargetRef{ID: t.ID, Name: t.Name})
	}
	out.Assigned = len(out.Targets) > 0
	return out, nil
}

// DeleteGroup deletes one Group. Assigned Groups are blocked unless
// unassign is set; that only removes Group Assignments. Managed Links stay
// until the next explicit Distribution.
func (a *App) DeleteGroup(_ context.Context, id int64, unassign bool) (*GroupDeleteResult, error) {
	preview, err := a.PreviewDeleteGroup(id)
	if err != nil {
		return nil, err
	}
	if preview.Assigned && !unassign {
		return nil, Errorf(CodeConflict, "Group %q is assigned to %d Target(s); pass --unassign to remove those Assignments first",
			preview.Group.Name, len(preview.Targets))
	}
	if unassign {
		if err := state.DeleteGroupAssignments(a.db, id); err != nil {
			return nil, Errorf(CodeInternal, "removing Assignments of Group %q: %v", preview.Group.Name, err)
		}
	}
	if err := state.DeleteGroup(a.db, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, Errorf(CodeNotFound, "Group %d not found", id)
		}
		return nil, Errorf(CodeInternal, "deleting Group %d: %v", id, err)
	}
	unassigned := preview.Targets
	if unassigned == nil {
		unassigned = []TargetRef{}
	}
	return &GroupDeleteResult{Group: preview.Group, Unassigned: unassigned}, nil
}
