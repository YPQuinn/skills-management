package app

import (
	"database/sql"
	"errors"
	"os"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// resolveRemoveIntent converges one unfinished remove. A persisted
// isolation directory is re-opened with O_NOFOLLOW and finished through
// that fd. A legacy intent with an empty slot_name recovers from the
// live Target slug and ledger (decision 06 closed set).
func (a *App) resolveRemoveIntent(it state.LinkIntent) error {
	site, err := a.loadIntentSite(it)
	if err != nil || site == nil {
		return err
	}
	ledger, err := state.GetManagedLink(a.db, it.TargetID, it.SkillID)
	if errors.Is(err, sql.ErrNoRows) {
		return state.DeleteLinkIntent(a.db, it.ID)
	}
	if err != nil {
		return Errorf(CodeInternal, "reading the Managed Link for intent %d: %v", it.ID, err)
	}
	if it.SlotName == "" {
		return a.resolveLegacyRemove(site, it, ledger)
	}
	return a.resolveIsolatedRemove(site, it, ledger)
}

func (a *App) resolveIsolatedRemove(site *intentSite, it state.LinkIntent, ledger *state.ManagedLink) error {
	kind, _, err := distribution.ProbeLink(site.target.Path, it.SlotName)
	if os.IsNotExist(err) {
		return a.continueRemoveFromSlug(site, it, ledger, false)
	}
	if err != nil {
		return nil
	}
	if kind != distribution.KindDir {
		return a.reportUnprovenIntent(it, "the persisted isolation path is not a proven directory")
	}
	_, _, entryErr := distribution.ProbeIsolationEntry(site.target.Path, it.SlotName)
	if os.IsNotExist(entryErr) {
		// Dir exists but IsolatedEntry does not: crash after mkdirat and
		// before IsolateInto. Do not treat this as RemoveAbsent.
		return a.continueRemoveFromSlug(site, it, ledger, true)
	}
	if entryErr != nil {
		return nil
	}
	return a.finishPreparedRemove(site, it, ledger)
}

func (a *App) continueRemoveFromSlug(site *intentSite, it state.LinkIntent, ledger *state.ManagedLink, reuseDir bool) error {
	kind, raw, err := distribution.ProbeLink(site.target.Path, site.slug)
	if os.IsNotExist(err) {
		if reuseDir {
			_ = distribution.DiscardIsolation(site.target.Path, it.SlotName)
		}
		return a.finalizeRemoveIntent(it)
	}
	if err != nil {
		return nil
	}
	if kind != distribution.KindSymlink || raw != ledger.RawTarget {
		if reuseDir {
			_ = distribution.DiscardIsolation(site.target.Path, it.SlotName)
		}
		return a.relinquishChangedEntry(it)
	}
	if !reuseDir {
		if err := distribution.CreateIsolationDir(site.target.Path, it.SlotName); err != nil {
			if errors.Is(err, distribution.ErrSlotOccupied) {
				return a.reportUnprovenIntent(it, "the persisted isolation directory is occupied")
			}
			return nil
		}
	}
	if err := distribution.IsolateInto(site.target.Path, site.slug, it.SlotName); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = distribution.DiscardIsolation(site.target.Path, it.SlotName)
			return a.finalizeRemoveIntent(it)
		}
		if errors.Is(err, distribution.ErrSlotOccupied) {
			return a.reportUnprovenIntent(it, "the isolation entry is occupied")
		}
		return nil
	}
	if err := state.SetLinkIntentPhase(a.db, it.ID, state.LinkPhasePrepared); err != nil {
		return Errorf(CodeInternal, "recording the isolated entry of intent %d: %v", it.ID, err)
	}
	return a.finishPreparedRemove(site, it, ledger)
}

func (a *App) resolveLegacyRemove(site *intentSite, it state.LinkIntent, ledger *state.ManagedLink) error {
	kind, raw, err := distribution.ProbeLink(site.target.Path, site.slug)
	if os.IsNotExist(err) {
		return a.finalizeRemoveIntent(it)
	}
	if err != nil {
		return nil
	}
	if kind != distribution.KindSymlink || raw != ledger.RawTarget {
		return a.relinquishChangedEntry(it)
	}
	isolation, err := distribution.NewIsolationName()
	if err != nil {
		return nil
	}
	if err := state.SetLinkIntentSlot(a.db, it.ID, isolation, state.LinkPhasePlanned); err != nil {
		return Errorf(CodeInternal, "persisting isolation for legacy intent %d: %v", it.ID, err)
	}
	it.SlotName = isolation
	return a.continueRemoveFromSlug(site, it, ledger, false)
}

func (a *App) finishPreparedRemove(site *intentSite, it state.LinkIntent, ledger *state.ManagedLink) error {
	rr, err := distribution.FinishIsolatedRemove(site.target.Path, site.slug, ledger.RawTarget, it.SlotName)
	if err != nil {
		return a.reportUnprovenIntent(it, "the isolated entry could not be verified: "+err.Error())
	}
	if rr == distribution.RemoveMismatch {
		return a.relinquishChangedEntry(it)
	}
	return a.finalizeRemoveIntent(it)
}

func (a *App) finalizeRemoveIntent(it state.LinkIntent) error {
	if err := state.FinalizeRemoveLedger(a.db, it.TargetID, it.SkillID, it.ID); err != nil {
		return Errorf(CodeInternal, "recovering remove intent %d: %v", it.ID, err)
	}
	return nil
}

func (a *App) relinquishChangedEntry(it state.LinkIntent) error {
	if err := state.FinalizeRemoveLedger(a.db, it.TargetID, it.SkillID, it.ID); err != nil {
		return Errorf(CodeInternal, "recovering remove intent %d: %v", it.ID, err)
	}
	result, msg := a.recordItemOutcome(&state.Target{ID: it.TargetID}, planItem{
		skillID: it.SkillID, desired: distribution.DesiredAbsent, observed: distribution.ObservedConflict,
	}, distribution.OutcomeOwnershipLost, distribution.ObservedConflict, "", time.Now().UTC())
	if result == distribution.OutcomeFailed {
		return Errorf(CodeInternal, "recording the relinquished claim of intent %d: %s", it.ID, msg)
	}
	return nil
}

func (a *App) reportUnprovenIntent(it state.LinkIntent, msg string) error {
	now := time.Now().UTC()
	if err := state.UpdateDistributionItemOutcome(a.db, it.TargetID, it.SkillID,
		distribution.DesiredAbsent, distribution.ObservedConflict,
		distribution.OutcomeFailed, msg, now); err != nil {
		return Errorf(CodeInternal, "reporting unproven intent %d: %v", it.ID, err)
	}
	return nil
}
