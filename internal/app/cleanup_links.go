package app

import (
	"context"
	"database/sql"
	"errors"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// removeManagedLinkLocked inspects one Target and removes the proven
// Managed Link for skillID. The caller holds the Store lock and this
// Target's exclusive lock. ownership_lost leaves the filesystem entry and
// is not a failure.
func (a *App) removeManagedLinkLocked(ctx context.Context, t *state.Target, skillID int64) (CleanupLink, error) {
	out := CleanupLink{TargetID: t.ID, TargetName: t.Name, SkillID: skillID}
	ledger, err := state.GetManagedLink(a.db, t.ID, skillID)
	if errors.Is(err, sql.ErrNoRows) {
		out.Result = distribution.OutcomeNoOp
		return out, nil
	}
	if err != nil {
		return out, Errorf(CodeInternal, "reading the Managed Link: %v", err)
	}
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		return out, Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
	}
	out.Slug = d.Skill.Slug

	ins, err := a.inspectTarget(ctx, t)
	if err != nil {
		return out, err
	}
	if ins.status.Stale {
		out.Result = distribution.OutcomeFailed
		out.Warning = ins.status.InspectionError
		return out, Errorf(CodeConflict, "cannot clean a Managed Link at Target %q: %s", t.Name, ins.status.InspectionError)
	}
	switch ins.gate.State {
	case distribution.StateRedirected, distribution.StateInvalid:
		out.Result = distribution.OutcomeFailed
		out.Warning = ins.gate.Error
		return out, Errorf(CodeConflict, "cannot clean a Managed Link at Target %q: %s", t.Name, ins.gate.Error)
	}

	var p *planItem
	for i := range ins.plan {
		if ins.plan[i].skillID == skillID {
			p = &ins.plan[i]
			break
		}
	}
	if p == nil {
		if err := state.DeleteManagedLink(a.db, t.ID, skillID); err != nil {
			return out, Errorf(CodeInternal, "discarding the ownership claim: %v", err)
		}
		out.Result = distribution.OutcomeRemoved
		return out, nil
	}
	_ = ledger
	result, errMsg := a.executeRemove(t, *p)
	out.Result = result
	out.Warning = errMsg
	if result == distribution.OutcomeFailed {
		return out, Errorf(CodeConflict, "safe removal of %q at Target %q failed: %s", out.Slug, t.Name, errMsg)
	}
	return out, nil
}

// removeAllManagedLinksLocked removes every ledger row of one Target after
// a coherent inspection. The caller holds the locks.
func (a *App) removeAllManagedLinksLocked(ctx context.Context, t *state.Target) ([]CleanupLink, error) {
	ins, err := a.inspectTarget(ctx, t)
	if err != nil {
		return nil, err
	}
	if ins.status.Stale {
		return nil, Errorf(CodeConflict, "cannot clean Target %q: %s", t.Name, ins.status.InspectionError)
	}
	switch ins.gate.State {
	case distribution.StateRedirected, distribution.StateInvalid:
		return nil, Errorf(CodeConflict, "cannot clean Target %q: %s", t.Name, ins.gate.Error)
	}

	var out []CleanupLink
	var first error
	for _, p := range ins.plan {
		if _, err := state.GetManagedLink(a.db, t.ID, p.skillID); errors.Is(err, sql.ErrNoRows) {
			continue
		} else if err != nil {
			return out, Errorf(CodeInternal, "reading the Managed Link: %v", err)
		}
		result, errMsg := a.executeRemove(t, p)
		cl := CleanupLink{
			TargetID: t.ID, TargetName: t.Name, SkillID: p.skillID, Slug: p.slug,
			Result: result, Warning: errMsg,
		}
		out = append(out, cl)
		if result == distribution.OutcomeFailed && first == nil {
			first = Errorf(CodeConflict, "safe removal of %q at Target %q failed: %s", p.slug, t.Name, errMsg)
		}
	}
	return out, first
}
