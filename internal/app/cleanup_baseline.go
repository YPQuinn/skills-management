package app

import (
	"database/sql"
	"errors"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

const baselineClearDigest = "cleared"

// runBaselineClear isolates the Baseline, commits Binding/digest/status,
// then drains the parked tree. pending recovery restores the Baseline;
// committed recovery finishes the drain.
func (a *App) runBaselineClear(d *state.SkillDetail, commit func(skillstore.Operation) error) error {
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		SkillID: d.Skill.ID, Slug: d.Skill.Slug, Kind: skillstore.KindBaseline,
		OldDigest: d.Skill.BaselineDigest, NewDigest: baselineClearDigest,
		BaselineMode: skillstore.BaselineClear, BaselineDigest: d.Skill.BaselineDigest,
	})
	if err != nil {
		return Errorf(CodeInternal, "persisting the Baseline-clear intent of Skill %q: %v", d.Skill.Slug, err)
	}
	if err := a.store.IsolateBaseline(op); err != nil {
		_ = a.store.RestoreBaseline(op)
		_ = a.abortParked(op)
		return Errorf(CodeInternal, "isolating the Baseline of Skill %q: %v", d.Skill.Slug, err)
	}
	if err := commit(op); err != nil {
		if rerr := a.store.RestoreBaseline(op); rerr != nil {
			return Errorf(CodeRecovery, "clearing the Baseline of Skill %q failed (%v) and the tree could not be restored: %v", d.Skill.Slug, err, rerr)
		}
		_ = a.abortParked(op)
		return err
	}
	op.Phase = skillstore.PhaseCommitted
	if err := a.store.FinalizeBaselineClear(op); err != nil {
		return Errorf(CodeRecovery, "committed Baseline clear of Skill %q could not drain: %v", d.Skill.Slug, err)
	}
	if err := state.DeleteCommittedParked(a.db, op); err != nil {
		return Errorf(CodeRecovery, "committed Baseline clear of Skill %q could not clear its journal: %v", d.Skill.Slug, err)
	}
	return nil
}

func (a *App) commitClear(op skillstore.Operation, skillID int64, oldBaseline string, b *state.Binding, status string) error {
	if a.commitBaselineClear != nil {
		return a.commitBaselineClear(op, skillID, oldBaseline, b, status)
	}
	return state.CommitBaselineClear(a.db, op, skillID, oldBaseline, b, status)
}

func (a *App) detachSkillLocked(skillID int64) error {
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if errors.Is(err, sql.ErrNoRows) {
		return Errorf(CodeNotFound, "Skill %d not found", skillID)
	}
	if err != nil {
		return Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
	}
	if d.Binding == nil {
		return a.persistSyncStatus(skillID, sync.StatusUnbound, false)
	}
	return a.runBaselineClear(d, func(op skillstore.Operation) error {
		if err := a.commitClear(op, d.Skill.ID, d.Skill.BaselineDigest, nil, string(sync.StatusUnbound)); err != nil {
			return Errorf(CodeInternal, "detaching Skill %q: %v", d.Skill.Slug, err)
		}
		return nil
	})
}
