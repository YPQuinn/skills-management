package app

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// resolveCreateIntent first retires only proven staging, then reconciles
// the live slug against the pre-publication proof. Unknown staging blocks
// recovery without deleting its receipt or claiming the live entry.
func (a *App) resolveCreateIntent(it state.LinkIntent) error {
	site, err := a.loadIntentSite(it)
	if err != nil || site == nil {
		return err
	}
	if it.SlotName != "" {
		if err := distribution.DiscardCreateStaging(site.target.Path, it.SlotName, intentProof(it)); err != nil {
			return Errorf(CodeRecovery, "preserving create intent %d staging %s: %v", it.ID, it.SlotName, err)
		}
	}
	got, err := distribution.ProbeSymlink(site.target.Path, site.slug)
	if errors.Is(err, os.ErrNotExist) {
		return state.DeleteLinkIntent(a.db, it.ID)
	}
	if err != nil {
		if errors.Is(err, distribution.ErrNotSymlink) || errors.Is(err, distribution.ErrEntryChanged) {
			return a.endCreateConflict(it)
		}
		return nil
	}
	proof := intentProof(it)
	if !proof.Proven() || !proof.Matches(got) {
		return a.endCreateConflict(it)
	}
	return a.finalizeCreateIntent(site, it, proof)
}

func (a *App) finalizeCreateIntent(site *intentSite, it state.LinkIntent, proof distribution.LinkProof) error {
	now := time.Now().UTC()
	if err := state.FinalizeCreateLedger(a.db, state.ManagedLink{
		TargetID: it.TargetID, SkillID: it.SkillID,
		LinkPath:  filepath.Join(site.target.Path, site.slug),
		RawTarget: proof.Raw, LinkDev: proof.Dev, LinkIno: proof.Ino, LinkMtime: proof.Mtime, EstablishedAt: now,
	}, it.ID); err != nil {
		return Errorf(CodeInternal, "recovering create intent %d: %v", it.ID, err)
	}
	return nil
}

func (a *App) endCreateConflict(it state.LinkIntent) error {
	now := time.Now().UTC()
	if err := state.DeleteLinkIntent(a.db, it.ID); err != nil {
		return Errorf(CodeInternal, "clearing conflicting create intent %d: %v", it.ID, err)
	}
	if err := state.UpdateDistributionItemOutcome(a.db, it.TargetID, it.SkillID,
		distribution.DesiredPresent, distribution.ObservedConflict,
		distribution.OutcomeBlockedConflict, "an entry appeared at the link path", now); err != nil {
		return Errorf(CodeInternal, "recording the create conflict of intent %d: %v", it.ID, err)
	}
	return nil
}
