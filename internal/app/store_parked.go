package app

import (
	"context"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
)

func parkedJournal(op skillstore.Operation) bool {
	return op.Kind == skillstore.KindRemove ||
		(op.Kind == skillstore.KindBaseline && op.BaselineMode == skillstore.BaselineClear)
}

// recoverParkedJournal converges a remove or Baseline-clear journal that
// parks trees instead of using the install-proof receipt protocol.
// pending restores the Store Skill; committed finishes the drain after
// the DB row (or Baseline digest) has already been committed.
func (a *App) recoverParkedJournal(ctx context.Context, op skillstore.Operation) error {
	switch op.Phase {
	case skillstore.PhasePending:
		if err := a.restoreParked(ctx, op); err != nil {
			return err
		}
		return a.deleteOpIntent(op)
	case skillstore.PhaseCommitted:
		if err := a.finalizeParked(ctx, op); err != nil {
			return err
		}
		return state.DeleteCommittedParked(a.db, op)
	default:
		return Errorf(CodeRecovery, "parked operation %d has phase %q", op.ID, op.Phase)
	}
}

func (a *App) restoreParked(ctx context.Context, op skillstore.Operation) error {
	if op.Kind == skillstore.KindRemove {
		return a.store.RestoreRemove(op)
	}
	return a.store.RestoreBaseline(op)
}

func (a *App) finalizeParked(ctx context.Context, op skillstore.Operation) error {
	if op.Kind == skillstore.KindRemove {
		return a.store.FinalizeRemove(op)
	}
	return a.store.FinalizeBaselineClear(op)
}
