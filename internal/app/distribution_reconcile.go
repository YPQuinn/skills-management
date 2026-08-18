package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// DistributeTarget reconciles one Target with its Assignments under the
// Store shared lock and the Target exclusive lock. A dry run performs the
// safety gate and the coherent inspection and returns the predicted plan
// without mutating; a real run then creates missing desired links before
// removing no-longer-desired Managed Links, each through a durable intent
// (decision 06). One item's conflict, broken link, or failure never
// prevents the others.
func (a *App) DistributeTarget(ctx context.Context, targetID int64, dryRun bool) (*DistributionResult, error) {
	t, err := state.GetTargetByID(a.db, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", targetID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", targetID, err)
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
	return a.distributeTargetLocked(ctx, t, dryRun)
}

// distributeTargetLocked runs the gate, one coherent inspection, and the
// reconciliation for one Target whose locks and intent recovery the caller
// already holds.
func (a *App) distributeTargetLocked(ctx context.Context, t *state.Target, dryRun bool) (*DistributionResult, error) {
	started := time.Now().UTC()
	result := &DistributionResult{TargetID: t.ID, DryRun: dryRun, Inspected: &started, Items: []DistributionItemResult{}}
	finish := func() (*DistributionResult, error) {
		if !dryRun {
			if err := state.UpdateTargetOutcome(a.db, t.ID, result.Outcome, started, time.Now().UTC(), result.Error); err != nil {
				return nil, Errorf(CodeInternal, "recording the distribution outcome of Target %d: %v", t.ID, err)
			}
		}
		return result, nil
	}

	ins, err := a.inspectTarget(ctx, t)
	if err != nil {
		return nil, err
	}
	result.Inspected = ins.status.InspectedAt
	result.Stale = ins.status.Stale

	// Before inspection becomes mutation, the gate runs again so a prefix
	// swap after the inspection can never redirect a write (decision 06).
	switch ins.gate.State {
	case distribution.StateRedirected, distribution.StateInvalid:
		result.Outcome = distribution.ResultFailed
		result.Error = ins.gate.Error
		return finish()
	case distribution.StateMissing, distribution.StateOK:
	default:
		result.Outcome = distribution.ResultFailed
		result.Error = fmt.Sprintf("unknown Target gate state %q", ins.gate.State)
		return finish()
	}
	if ins.status.Stale {
		result.Outcome = distribution.ResultFailed
		result.Error = ins.status.InspectionError
		return finish()
	}

	planned := make([]string, 0, len(ins.plan))
	for _, p := range ins.plan {
		action := distribution.PlanItem(p.desired, p.observed, p.storeOK)
		planned = append(planned, action)
		result.Items = append(result.Items, DistributionItemResult{
			SkillID: p.skillID, Slug: p.slug, Desired: p.desired, Observed: p.observed,
			Action: action, Result: action,
		})
	}
	result.Outcome = distribution.TargetOutcome(planned)
	result.Summary.Total = len(planned)
	for _, r := range planned {
		result.Summary.add(r)
	}
	if dryRun {
		return result, nil
	}

	// Creates first, then removals, each independently (decision 06). A
	// cancelled request stops before each next item without rolling back
	// completed ones.
	for i := range result.Items {
		item := &result.Items[i]
		p := ins.plan[i]
		if item.Action != distribution.OutcomeCreated {
			continue
		}
		if err := ctx.Err(); err != nil {
			item.Result, item.Error = a.failItem(t, p, cancelledMessage(err), time.Now().UTC())
			continue
		}
		item.Result, item.Error = a.executeCreate(t, p)
	}
	for i := range result.Items {
		item := &result.Items[i]
		p := ins.plan[i]
		switch item.Action {
		case distribution.OutcomeRemoved, distribution.OutcomeOwnershipLost:
			if err := ctx.Err(); err != nil {
				item.Result, item.Error = a.failItem(t, p, cancelledMessage(err), time.Now().UTC())
				continue
			}
			item.Result, item.Error = a.executeRemove(t, p)
		}
	}
	results := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		results = append(results, item.Result)
	}
	result.Outcome = distribution.TargetOutcome(results)
	result.Summary = DistributionSummary{Total: len(results)}
	for _, r := range results {
		result.Summary.add(r)
	}
	// Persist one complete observation after the mutations so the stored
	// view matches the filesystem (managed, raw, resolved), not the
	// per-item outcome patch.
	if _, err := a.inspectTarget(ctx, t); err != nil {
		return nil, err
	}
	return finish()
}

// cancelledMessage explains one request-cancellation failure.
func cancelledMessage(err error) string {
	return "the request was cancelled: " + err.Error()
}

// recordItemOutcome persists one item's latest outcome on its observation
// row. A relation that ended up absent and missing has nothing left to
// observe, so its row is removed.
func (a *App) recordItemOutcome(t *state.Target, p planItem, result, observed, errMsg string, at time.Time) (string, string) {
	if p.desired == distribution.DesiredAbsent && observed == distribution.ObservedMissing && result == distribution.OutcomeRemoved {
		if err := state.DeleteDistributionItem(a.db, t.ID, p.skillID); err != nil {
			return distribution.OutcomeFailed, "recording the item outcome: " + err.Error()
		}
		return result, errMsg
	}
	if err := state.UpdateDistributionItemOutcome(a.db, t.ID, p.skillID, p.desired, observed, result, errMsg, at); err != nil {
		return distribution.OutcomeFailed, "recording the item outcome: " + err.Error()
	}
	return result, errMsg
}
