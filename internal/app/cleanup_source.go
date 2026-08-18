package app

import (
	"context"
	"database/sql"
	"errors"

	"skillctl/internal/state"
)

// PreviewDeleteSource reports the bound Skills that block Source deletion
// unless the caller explicitly detaches them.
func (a *App) PreviewDeleteSource(id int64) (*SourceDeletePreview, error) {
	src, err := a.ShowSource(id)
	if err != nil {
		return nil, err
	}
	details, err := state.ListSkillsBySource(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Skills bound to Source %d: %v", id, err)
	}
	out := &SourceDeletePreview{
		SourceID: src.ID, Name: src.Name, BoundSkills: []SkillRef{},
	}
	for _, d := range details {
		out.BoundSkills = append(out.BoundSkills, SkillRef{ID: d.Skill.ID, Slug: d.Skill.Slug, Name: d.Skill.Name})
	}
	out.RequiresDetach = len(out.BoundSkills) > 0
	return out, nil
}

// DeleteSource deletes one Source. Bound Skills block the operation unless
// detachSkills is set; those Skills are detached and never deleted.
func (a *App) DeleteSource(ctx context.Context, id int64, detachSkills bool) (*SourceDeleteResult, error) {
	src, err := a.ShowSource(id)
	if err != nil {
		return nil, err
	}
	var detached []SkillRef
	if err := a.withStoreLock(ctx, func() error {
		bound, err := state.ListSkillsBySource(a.db, id)
		if err != nil {
			return Errorf(CodeInternal, "listing Skills bound to Source %d: %v", id, err)
		}
		if len(bound) > 0 && !detachSkills {
			return Errorf(CodeConflict, "Source %q still has %d bound Skill(s); pass --detach-skills to detach them first",
				src.Name, len(bound))
		}
		for _, d := range bound {
			if d.Binding == nil || d.Binding.SourceID != id {
				continue
			}
			if err := a.detachSkillLocked(d.Skill.ID); err != nil {
				return err
			}
			detached = append(detached, SkillRef{ID: d.Skill.ID, Slug: d.Skill.Slug, Name: d.Skill.Name})
		}
		if err := state.DeleteSource(a.db, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Errorf(CodeNotFound, "Source %d not found", id)
			}
			return Errorf(CodeInternal, "deleting Source %d: %v", id, err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if detached == nil {
		detached = []SkillRef{}
	}
	return &SourceDeleteResult{SourceID: src.ID, Name: src.Name, Detached: detached}, nil
}
