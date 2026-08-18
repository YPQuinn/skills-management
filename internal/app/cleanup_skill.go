package app

import (
	"context"
	"database/sql"
	"errors"

	"skillctl/internal/state"
)

// PreviewDeleteSkill reports Groups, desired Targets, and Managed Links
// that make a Skill referenced.
func (a *App) PreviewDeleteSkill(id int64) (*SkillDeletePreview, error) {
	d, err := state.GetSkillDetailByID(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", id)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", id, err)
	}
	impact, err := a.replaceImpact(id)
	if err != nil {
		return nil, Errorf(CodeInternal, "reading the delete impact: %v", err)
	}
	refs, err := state.ListManagedLinksBySkill(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Managed Links of Skill %d: %v", id, err)
	}
	out := &SkillDeletePreview{
		Skill:   SkillRef{ID: d.Skill.ID, Slug: d.Skill.Slug, Name: d.Skill.Name},
		Groups:  impact.Groups,
		Targets: impact.Targets,
		Links:   cleanupLinksFrom(refs),
	}
	out.Referenced = len(out.Targets) > 0 || len(out.Links) > 0
	return out, nil
}

// DetachSkill removes the Source Binding and Baseline. The Skill, Groups,
// Assignments, and distributed links stay.
func (a *App) DetachSkill(ctx context.Context, id int64) (*Skill, error) {
	if _, err := state.GetSkillDetailByID(a.db, id); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", id)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", id, err)
	}
	if err := a.withStoreLock(ctx, func() error {
		return a.detachSkillLocked(id)
	}); err != nil {
		return nil, err
	}
	return a.ShowSkill(id)
}

// DeleteSkill deletes one Skill. Referenced Skills (desired at a Target or
// carrying a Managed Link) require cleanup, which removes related
// Assignments and verifiable Managed Links first. A cleanup failure
// retains the Store Skill. Desired Targets, Managed Links, and open
// link intents are collected and recovered under the Store lock.
func (a *App) DeleteSkill(ctx context.Context, id int64, cleanup bool) (*SkillDeleteResult, error) {
	result := &SkillDeleteResult{Links: []CleanupLink{}}
	if err := a.withSkillCleanupLocks(ctx, id, func() error {
		preview, err := a.PreviewDeleteSkill(id)
		if err != nil {
			return err
		}
		result.Skill = preview.Skill
		if preview.Referenced && !cleanup {
			return Errorf(CodeConflict, "Skill %q is still referenced by %d Target(s) or Managed Link(s); pass --cleanup to remove them first",
				preview.Skill.Slug, len(preview.Targets)+len(preview.Links))
		}
		for _, l := range preview.Links {
			t, err := state.GetTargetByID(a.db, l.TargetID)
			if err != nil {
				return Errorf(CodeInternal, "reading Target %d: %v", l.TargetID, err)
			}
			cl, err := a.removeManagedLinkLocked(ctx, t, id)
			result.Links = append(result.Links, cl)
			if err != nil {
				return err
			}
		}
		as, err := state.ListSkillAssignments(a.db, id)
		if err != nil {
			return Errorf(CodeInternal, "listing Assignments of Skill %d: %v", id, err)
		}
		if err := state.DeleteSkillAssignments(a.db, id); err != nil {
			return Errorf(CodeInternal, "removing Assignments of Skill %q: %v", preview.Skill.Slug, err)
		}
		result.RemovedAssignments = len(as)
		if err := state.DeleteSkillGroupMemberships(a.db, id); err != nil {
			return Errorf(CodeInternal, "removing Group memberships of Skill %q: %v", preview.Skill.Slug, err)
		}
		result.RemovedMemberships = len(preview.Groups)
		leftover, err := state.ListManagedLinksBySkill(a.db, id)
		if err != nil {
			return Errorf(CodeInternal, "re-reading Managed Links of Skill %d: %v", id, err)
		}
		intents, err := state.ListOpenLinkIntentsBySkill(a.db, id)
		if err != nil {
			return Errorf(CodeInternal, "re-reading link intents of Skill %d: %v", id, err)
		}
		if len(leftover) > 0 || len(intents) > 0 {
			return Errorf(CodeConflict, "Skill %q still has Managed Links or unfinished link intents; the Store copy was kept", preview.Skill.Slug)
		}
		return a.removeSkillJournaled(preview.Skill.Slug, id)
	}); err != nil {
		return nil, err
	}
	return result, nil
}
