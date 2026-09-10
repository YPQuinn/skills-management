package app

import (
	"context"
	"database/sql"
	"errors"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// LinkTargetSkill establishes the Managed Link for one desired Skill at one
// Target, without reconciling the rest. It reuses the same gate, coherent
// inspection, and durable create primitive as a full distribution, so the
// ownership and no-overwrite guarantees are identical. Linking an
// already-linked Skill is a no-op. It returns the fresh Distribution Status.
func (a *App) LinkTargetSkill(ctx context.Context, targetID, skillID int64) (*DistributionStatus, error) {
	t, err := a.lockedTargetForLink(targetID, skillID)
	if err != nil {
		return nil, err
	}
	held, err := a.acquireStoreSharedLock()
	if err != nil {
		return nil, err
	}
	defer held.Unlock()
	targetHeld, err := a.acquireTargetLock(t.ID)
	if err != nil {
		return nil, err
	}
	defer targetHeld.Unlock()
	if err := a.resolveTargetIntents(t.ID); err != nil {
		return nil, err
	}
	return a.mutateTargetLink(ctx, t, skillID, true)
}

// UnlinkTargetSkill removes the Managed Link for one Skill at one Target
// while leaving the Assignment in place, so the Skill stays desired and a
// later distribution would recreate it. It only ever removes a link Skill
// Manager owns; a foreign entry is never touched. Unlinking an
// already-missing link is a no-op. It returns the fresh Distribution Status.
func (a *App) UnlinkTargetSkill(ctx context.Context, targetID, skillID int64) (*DistributionStatus, error) {
	t, err := a.lockedTargetForLink(targetID, skillID)
	if err != nil {
		return nil, err
	}
	held, err := a.acquireStoreSharedLock()
	if err != nil {
		return nil, err
	}
	defer held.Unlock()
	targetHeld, err := a.acquireTargetLock(t.ID)
	if err != nil {
		return nil, err
	}
	defer targetHeld.Unlock()
	if err := a.resolveTargetIntents(t.ID); err != nil {
		return nil, err
	}
	return a.mutateTargetLink(ctx, t, skillID, false)
}

// lockedTargetForLink resolves the Target and confirms the Skill exists,
// mapping missing rows to CodeNotFound before any lock is taken.
func (a *App) lockedTargetForLink(targetID, skillID int64) (*state.Target, error) {
	t, err := state.GetTargetByID(a.db, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", targetID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", targetID, err)
	}
	if _, err := state.GetSkillDetailByID(a.db, skillID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", skillID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
	}
	return t, nil
}

// mutateTargetLink runs one coherent inspection under the held locks, then
// creates or removes exactly one relation's link, returning a fresh status.
func (a *App) mutateTargetLink(ctx context.Context, t *state.Target, skillID int64, create bool) (*DistributionStatus, error) {
	ins, err := a.inspectTarget(ctx, t)
	if err != nil {
		return nil, err
	}
	switch ins.gate.State {
	case distribution.StateRedirected, distribution.StateInvalid:
		return nil, Errorf(CodeTargetConflict, "the Target cannot be modified: %s", ins.gate.Error)
	case distribution.StateMissing, distribution.StateOK:
		// Allowed: like a full distribution, creating a link may create the
		// missing container; removing one is a no-op when nothing is there.
	default:
		return nil, Errorf(CodeTargetConflict, "unknown Target gate state %q", ins.gate.State)
	}
	if ins.status.Stale {
		return nil, Errorf(CodeTargetConflict, "the Target observation is stale: %s", ins.status.InspectionError)
	}

	var p *planItem
	for i := range ins.plan {
		if ins.plan[i].skillID == skillID {
			p = &ins.plan[i]
			break
		}
	}
	if p == nil {
		return nil, Errorf(CodeTargetConflict, "Skill %d is not assigned to Target %q", skillID, t.Name)
	}

	if create {
		if err := a.linkOne(t, *p); err != nil {
			return nil, err
		}
	} else if p.observed != distribution.ObservedMissing {
		if result, msg := a.executeRemove(t, *p); result == distribution.OutcomeFailed {
			return nil, Errorf(CodeInternal, "%s", msg)
		}
	}

	fresh, err := a.inspectTarget(ctx, t)
	if err != nil {
		return nil, err
	}
	return fresh.status, nil
}

// linkOne creates one relation's Managed Link, mapping a blocked create to a
// Target conflict the caller can present.
func (a *App) linkOne(t *state.Target, p planItem) error {
	switch distribution.PlanItem(p.desired, p.observed, p.storeOK) {
	case distribution.OutcomeNoOp:
		return nil // already linked
	case distribution.OutcomeCreated:
		if result, msg := a.executeCreate(t, p); result != distribution.OutcomeCreated {
			return linkBlockedError(result, msg)
		}
		return nil
	case distribution.OutcomeBlockedConflict:
		return Errorf(CodeTargetConflict, "an entry already exists at the link path for %q", p.slug)
	case distribution.OutcomeBlockedBroken:
		return Errorf(CodeTargetConflict, "the link for %q cannot be created: %s", p.slug, p.storeError)
	default:
		return Errorf(CodeTargetConflict, "the link for %q cannot be created", p.slug)
	}
}

// linkBlockedError maps a non-created create outcome to a client error.
func linkBlockedError(result, msg string) error {
	switch result {
	case distribution.OutcomeBlockedConflict, distribution.OutcomeBlockedBroken, distribution.OutcomeOwnershipLost:
		return Errorf(CodeTargetConflict, "%s", msg)
	default:
		return Errorf(CodeInternal, "%s", msg)
	}
}
