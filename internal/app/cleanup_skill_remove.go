package app

import (
	"context"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
)

const removeDigestAbsent = "absent"

// removeSkillJournaled isolates the Store Skill, commits the Skill-row
// delete, then drains the parked trees. A pending crash restores live so
// the DB never points at a missing tree; a committed crash finishes the
// drain with the row already gone.
func (a *App) removeSkillJournaled(slug string, skillID int64) error {
	live, _, _ := a.storeTreeState(context.Background(), slug)
	if live == "" {
		live = removeDigestAbsent
	}
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		SkillID: skillID, Slug: slug, Kind: skillstore.KindRemove,
		OldDigest: live, NewDigest: live,
	})
	if err != nil {
		return Errorf(CodeInternal, "persisting the remove intent of Skill %q: %v", slug, err)
	}
	if err := a.store.IsolateRemove(op); err != nil {
		_ = a.store.RestoreRemove(op)
		_ = a.abortParked(op)
		return Errorf(CodeInternal, "isolating Store content of Skill %q: %v", slug, err)
	}
	if a.afterSkillStoreRemoved != nil {
		a.afterSkillStoreRemoved()
	}
	if err := a.commitRemove(op, skillID); err != nil {
		if rerr := a.store.RestoreRemove(op); rerr != nil {
			return Errorf(CodeRecovery, "deleting Skill %q failed (%v) and the Store Skill could not be restored: %v", slug, err, rerr)
		}
		_ = a.abortParked(op)
		return Errorf(CodeInternal, "deleting Skill %q: %v", slug, err)
	}
	op.SkillID = 0
	op.Phase = skillstore.PhaseCommitted
	if err := a.store.FinalizeRemove(op); err != nil {
		return Errorf(CodeRecovery, "committed remove of Skill %q could not drain parked trees: %v", slug, err)
	}
	if err := state.DeleteCommittedParked(a.db, op); err != nil {
		return Errorf(CodeRecovery, "committed remove of Skill %q could not clear its journal: %v", slug, err)
	}
	return nil
}

func (a *App) commitRemove(op skillstore.Operation, skillID int64) error {
	if a.commitSkillDelete != nil {
		return a.commitSkillDelete(op, skillID)
	}
	return state.CommitSkillDelete(a.db, op, skillID)
}

func (a *App) abortParked(op skillstore.Operation) error {
	return state.DeleteOperationIntent(a.db, op)
}
