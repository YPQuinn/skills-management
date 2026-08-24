package app

import (
	"database/sql"
	"errors"
	"os"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// executeCreate records a planned intent and writes the link with one
// atomic no-overwrite symlinkat on the pinned Target fd.
func (a *App) executeCreate(t *state.Target, p planItem) (result, errMsg string) {
	now := time.Now().UTC()
	linkPath := distribution.ExpectedPath(t.Path, p.slug)
	intentID, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: t.ID, SkillID: p.skillID, Action: "create",
		LinkPath: linkPath, RawTarget: p.rawTarget,
		Phase: state.LinkPhasePlanned, CreatedAt: now,
	})
	if err != nil {
		return a.failItem(t, p, "recording the link intent: "+err.Error(), now)
	}
	proof, err := distribution.CreateLink(t.Path, p.slug, p.rawTarget)
	if err != nil {
		if derr := state.DeleteLinkIntent(a.db, intentID); derr != nil {
			return a.failItem(t, p, "clearing the link intent: "+derr.Error(), now)
		}
		if errors.Is(err, distribution.ErrEntryExists) {
			return a.recordItemOutcome(t, p, distribution.OutcomeBlockedConflict, distribution.ObservedConflict,
				"an entry appeared at the link path", now)
		}
		return a.failItem(t, p, "creating the link: "+err.Error(), now)
	}
	if err := state.SetLinkIntentIdentity(a.db, intentID, proof.Dev, proof.Ino, proof.Mtime); err != nil {
		return a.failItem(t, p, "recording the created link identity: "+err.Error(), now)
	}
	if a.linkMutateHook != nil {
		a.linkMutateHook("create")
	}
	got, err := distribution.ProbeSymlink(t.Path, p.slug)
	if err != nil || !proof.Matches(got) {
		if derr := state.DeleteLinkIntent(a.db, intentID); derr != nil {
			return a.failItem(t, p, "clearing the replaced link intent: "+derr.Error(), now)
		}
		if errors.Is(err, os.ErrNotExist) {
			return a.failItem(t, p, "the created link disappeared", now)
		}
		return a.recordItemOutcome(t, p, distribution.OutcomeBlockedConflict, distribution.ObservedConflict,
			"the created link was replaced", now)
	}
	if err := state.FinalizeCreateLedger(a.db, state.ManagedLink{
		TargetID: t.ID, SkillID: p.skillID, LinkPath: linkPath,
		RawTarget: proof.Raw, LinkDev: proof.Dev, LinkIno: proof.Ino, LinkMtime: proof.Mtime, EstablishedAt: now,
	}, intentID); err != nil {
		return a.failItem(t, p, "finalizing the link: "+err.Error(), now)
	}
	return a.recordItemOutcome(t, p, distribution.OutcomeCreated, distribution.ObservedLinked, "", now)
}

// executeRemove isolates the slug into the intent's private 0700 directory,
// verifies through that pinned fd, and unlinks only IsolatedEntry.
func (a *App) executeRemove(t *state.Target, p planItem) (result, errMsg string) {
	now := time.Now().UTC()
	ledger, err := state.GetManagedLink(a.db, t.ID, p.skillID)
	if errors.Is(err, sql.ErrNoRows) {
		return a.recordItemOutcome(t, p, distribution.OutcomeNoOp, p.observed, "", now)
	}
	if err != nil {
		return a.failItem(t, p, "reading the Managed Link: "+err.Error(), now)
	}
	if p.observed == distribution.ObservedConflict || p.observed == distribution.ObservedMissing {
		if err := state.DeleteManagedLink(a.db, t.ID, p.skillID); err != nil {
			return a.failItem(t, p, "discarding the ownership claim: "+err.Error(), now)
		}
		if p.observed == distribution.ObservedConflict {
			return a.recordItemOutcome(t, p, distribution.OutcomeOwnershipLost, p.observed, "", now)
		}
		return a.recordItemOutcome(t, p, distribution.OutcomeRemoved, p.observed, "", now)
	}
	proof := ledgerProof(ledger)
	if !proof.Proven() {
		return a.failItem(t, p, "the Managed Link identity is unproven", now)
	}
	isolation, err := distribution.NewIsolationName()
	if err != nil {
		return a.failItem(t, p, "allocating the isolation directory: "+err.Error(), now)
	}
	intentID, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: t.ID, SkillID: p.skillID, Action: "remove",
		LinkPath: ledger.LinkPath, RawTarget: proof.Raw,
		LinkDev: proof.Dev, LinkIno: proof.Ino, LinkMtime: proof.Mtime,
		SlotName: isolation, Phase: state.LinkPhasePlanned, CreatedAt: now,
	})
	if err != nil {
		return a.failItem(t, p, "recording the link intent: "+err.Error(), now)
	}
	if err := distribution.VerifyLink(t.Path, p.slug, proof); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := state.FinalizeRemoveLedger(a.db, t.ID, p.skillID, intentID); err != nil {
				return a.failItem(t, p, "finalizing the removal: "+err.Error(), now)
			}
			return a.recordItemOutcome(t, p, distribution.OutcomeRemoved, distribution.ObservedMissing, "", now)
		}
		if errors.Is(err, distribution.ErrLinkMismatch) {
			if err := state.FinalizeRemoveLedger(a.db, t.ID, p.skillID, intentID); err != nil {
				return a.failItem(t, p, "discarding the ownership claim: "+err.Error(), now)
			}
			return a.recordItemOutcome(t, p, distribution.OutcomeOwnershipLost, distribution.ObservedConflict, "", now)
		}
		return a.failItem(t, p, "verifying the Managed Link: "+err.Error(), now)
	}
	if err := distribution.CreateIsolationDir(t.Path, isolation); err != nil {
		return a.failItem(t, p, "creating the isolation directory: "+err.Error(), now)
	}
	if err := distribution.IsolateInto(t.Path, p.slug, isolation); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = distribution.DiscardIsolation(t.Path, isolation)
			if err := state.FinalizeRemoveLedger(a.db, t.ID, p.skillID, intentID); err != nil {
				return a.failItem(t, p, "finalizing the removal: "+err.Error(), now)
			}
			return a.recordItemOutcome(t, p, distribution.OutcomeRemoved, distribution.ObservedMissing, "", now)
		}
		return a.failItem(t, p, "isolating the link: "+err.Error(), now)
	}
	if a.linkMutateHook != nil {
		a.linkMutateHook("remove")
	}
	if err := state.SetLinkIntentPhase(a.db, intentID, state.LinkPhasePrepared); err != nil {
		return a.failItem(t, p, "recording the isolated entry: "+err.Error(), now)
	}
	return a.finishRemove(t, p, proof, isolation, intentID, now)
}

func (a *App) finishRemove(t *state.Target, p planItem, proof distribution.LinkProof, isolation string, intentID int64, now time.Time) (string, string) {
	rr, rerr := distribution.FinishIsolatedRemove(t.Path, p.slug, proof, isolation)
	if rerr != nil {
		return a.failItem(t, p, "removing the link: "+rerr.Error(), now)
	}
	switch rr {
	case distribution.RemoveMismatch:
		if err := state.FinalizeRemoveLedger(a.db, t.ID, p.skillID, intentID); err != nil {
			return a.failItem(t, p, "discarding the ownership claim: "+err.Error(), now)
		}
		return a.recordItemOutcome(t, p, distribution.OutcomeOwnershipLost, distribution.ObservedConflict, "", now)
	case distribution.RemoveDone, distribution.RemoveAbsent:
		if err := state.FinalizeRemoveLedger(a.db, t.ID, p.skillID, intentID); err != nil {
			return a.failItem(t, p, "finalizing the removal: "+err.Error(), now)
		}
		return a.recordItemOutcome(t, p, distribution.OutcomeRemoved, distribution.ObservedMissing, "", now)
	default:
		return a.failItem(t, p, "unknown removal outcome", now)
	}
}

// failItem records a failed item outcome with a CHECK-valid observed
// state. The error belongs in last_error, never in observed.
func (a *App) failItem(t *state.Target, p planItem, errMsg string, now time.Time) (string, string) {
	observed := p.observed
	switch observed {
	case distribution.ObservedLinked, distribution.ObservedMissing, distribution.ObservedConflict, distribution.ObservedBrokenLink:
	default:
		observed = distribution.ObservedMissing
	}
	return a.recordItemOutcome(t, p, distribution.OutcomeFailed, observed, errMsg, now)
}

func ledgerProof(l *state.ManagedLink) distribution.LinkProof {
	return distribution.LinkProof{Raw: l.RawTarget, Dev: l.LinkDev, Ino: l.LinkIno, Mtime: l.LinkMtime}
}

func intentProof(it state.LinkIntent) distribution.LinkProof {
	return distribution.LinkProof{Raw: it.RawTarget, Dev: it.LinkDev, Ino: it.LinkIno, Mtime: it.LinkMtime}
}
