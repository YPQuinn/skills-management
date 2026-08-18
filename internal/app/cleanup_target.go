package app

import (
	"context"
	"database/sql"
	"errors"

	"skillctl/internal/state"
)

// PreviewDeleteTarget reports Assignments and Managed Links that Target
// deletion would clean up. The container is never deleted.
func (a *App) PreviewDeleteTarget(id int64) (*TargetDeletePreview, error) {
	t, err := state.GetTargetByID(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", id)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", id, err)
	}
	as, err := state.ListTargetAssignmentDetails(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Assignments of Target %d: %v", id, err)
	}
	refs, err := state.ListManagedLinksByTarget(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Managed Links of Target %d: %v", id, err)
	}
	out := &TargetDeletePreview{
		Target:      TargetRef{ID: t.ID, Name: t.Name},
		Assignments: make([]AssignmentView, 0, len(as)),
		Links:       make([]CleanupLink, 0, len(refs)),
	}
	for _, d := range as {
		out.Assignments = append(out.Assignments, assignmentView(d))
	}
	for _, l := range refs {
		out.Links = append(out.Links, CleanupLink{
			TargetID: t.ID, TargetName: t.Name, SkillID: l.SkillID, Slug: l.Slug,
		})
	}
	return out, nil
}

// DeleteTarget removes every verifiable Managed Link and then the Target
// registration. The container remains. A failed safe removal retains the
// Target so the operation can be retried.
func (a *App) DeleteTarget(ctx context.Context, id int64) (*TargetDeleteResult, error) {
	preview, err := a.PreviewDeleteTarget(id)
	if err != nil {
		return nil, err
	}
	t, err := state.GetTargetByID(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", id, err)
	}
	result := &TargetDeleteResult{Target: preview.Target, Links: []CleanupLink{}}
	held, err := a.acquireStoreSharedLock()
	if err != nil {
		return nil, err
	}
	defer held.Unlock()
	targetHeld, err := a.acquireTargetLock(id)
	if err != nil {
		return nil, err
	}
	defer targetHeld.Unlock()
	if err := a.resolveTargetIntents(id); err != nil {
		return nil, err
	}
	links, err := a.removeAllManagedLinksLocked(ctx, t)
	result.Links = links
	if err != nil {
		return nil, err
	}
	if err := state.DeleteTarget(a.db, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, Errorf(CodeNotFound, "Target %d not found", id)
		}
		return nil, Errorf(CodeInternal, "deleting Target %d: %v", id, err)
	}
	return result, nil
}
