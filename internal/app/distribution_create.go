package app

import (
	"errors"
	"os"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

func (a *App) executeCreate(t *state.Target, p planItem) (result, errMsg string) {
	now := time.Now().UTC()
	isolation, err := distribution.NewIsolationName()
	if err != nil {
		return a.failItem(t, p, "allocating create staging: "+err.Error(), now)
	}
	linkPath := distribution.ExpectedPath(t.Path, p.slug)
	intentID, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: t.ID, SkillID: p.skillID, Action: "create",
		LinkPath: linkPath, RawTarget: p.rawTarget, SlotName: isolation,
		Phase: state.LinkPhasePlanned, CreatedAt: now,
	})
	if err != nil {
		return a.failItem(t, p, "recording the link intent: "+err.Error(), now)
	}
	if a.linkMutateHook != nil {
		a.linkMutateHook("create-planned")
	}
	proof, createErr := distribution.CreateLink(t.Path, p.slug, p.rawTarget, isolation, func(proof distribution.LinkProof) error {
		if a.linkMutateHook != nil {
			a.linkMutateHook("create-staged")
		}
		if err := state.SetLinkIntentIdentity(a.db, intentID, proof.Dev, proof.Ino, proof.Mtime); err != nil {
			return err
		}
		if a.linkMutateHook != nil {
			a.linkMutateHook("create-prepared")
		}
		return nil
	})
	if createErr == nil && a.linkMutateHook != nil {
		a.linkMutateHook("create")
	}
	if !errors.Is(createErr, distribution.ErrSlotOccupied) {
		if err := distribution.DiscardCreateStaging(t.Path, isolation, proof); err != nil {
			return a.failItem(t, p, "preserving unverified create staging: "+err.Error(), now)
		}
	}
	if createErr != nil {
		if err := state.DeleteLinkIntent(a.db, intentID); err != nil {
			return a.failItem(t, p, "clearing the link intent: "+err.Error(), now)
		}
		if errors.Is(createErr, distribution.ErrEntryExists) || errors.Is(createErr, distribution.ErrLinkMismatch) {
			return a.recordItemOutcome(t, p, distribution.OutcomeBlockedConflict, distribution.ObservedConflict,
				"an entry appeared or changed at the link path", now)
		}
		return a.failItem(t, p, "creating the link: "+createErr.Error(), now)
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
