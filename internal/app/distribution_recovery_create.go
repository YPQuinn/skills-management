package app

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// resolveCreateIntent converges one unfinished create from the final slug
// only: missing clears the intent; the exact expected symlink completes
// the ledger; any other entry is preserved and the intent ends as a
// conflict. Create uses no visible slot (decision 06 / symlinkat).
func (a *App) resolveCreateIntent(it state.LinkIntent) error {
	site, err := a.loadIntentSite(it)
	if err != nil || site == nil {
		return err
	}
	got, err := distribution.ProbeSymlink(site.target.Path, site.slug)
	if os.IsNotExist(err) {
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
