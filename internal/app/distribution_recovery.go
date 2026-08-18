package app

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"skillctl/internal/state"
)

// recoverLinkIntents resolves every unfinished link intent when the App
// opens, so each process observes a converged Target before its next
// write (decision 08). Targets are visited in stable Target-ID order;
// each visit holds that Target's exclusive lock, and a Target already
// being reconciled by another process refuses the open with CodeLocked.
func (a *App) recoverLinkIntents() error {
	intents, err := state.ListOpenLinkIntents(a.db)
	if err != nil {
		return Errorf(CodeInternal, "listing link intents: %v", err)
	}
	byTarget := map[int64]bool{}
	for _, it := range intents {
		byTarget[it.TargetID] = true
	}
	ids := make([]int64, 0, len(byTarget))
	for id := range byTarget {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) == 0 {
		return nil
	}
	storeHeld, err := a.acquireStoreSharedLock()
	if err != nil {
		return err
	}
	defer storeHeld.Unlock()
	for _, id := range ids {
		held, err := a.acquireTargetLock(id)
		if err != nil {
			return err
		}
		err = a.resolveTargetIntents(id)
		if err == nil {
			err = a.persistRecoveredObservation(id)
		}
		held.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

// persistRecoveredObservation writes one coherent observation after
// recovery mutated a Target, re-reading the Target so a stale intent
// path cannot steer the inspect.
func (a *App) persistRecoveredObservation(targetID int64) error {
	t, err := state.GetTargetByID(a.db, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return Errorf(CodeInternal, "re-reading Target %d: %v", targetID, err)
	}
	_, err = a.inspectTarget(context.Background(), t)
	return err
}

// intentSite is the live Target path and Skill slug for one intent. Recovery
// always re-reads both; it never uses intent.LinkPath as a filesystem path.
type intentSite struct {
	target    *state.Target
	slug      string
	storeRoot string
}

// loadIntentSite re-reads the Target and Skill named by one intent. A
// vanished Target or Skill clears the orphaned intent and returns a nil
// site so recovery does not fall back to the recorded link path.
func (a *App) loadIntentSite(it state.LinkIntent) (*intentSite, error) {
	t, err := state.GetTargetByID(a.db, it.TargetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, state.DeleteLinkIntent(a.db, it.ID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "re-reading Target %d for intent %d: %v", it.TargetID, it.ID, err)
	}
	d, err := state.GetSkillDetailByID(a.db, it.SkillID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, state.DeleteLinkIntent(a.db, it.ID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "re-reading Skill %d for intent %d: %v", it.SkillID, it.ID, err)
	}
	root, err := a.storeLinkRoot()
	if err != nil {
		return nil, err
	}
	return &intentSite{target: t, slug: d.Skill.Slug, storeRoot: root}, nil
}

// resolveTargetIntents resolves every unfinished intent of one Target
// deterministically (decision 06); the caller holds the Target's exclusive
// lock. A transient filesystem failure keeps the intent so the next open
// or reconciliation retries it.
func (a *App) resolveTargetIntents(targetID int64) error {
	intents, err := state.ListOpenLinkIntentsByTarget(a.db, targetID)
	if err != nil {
		return Errorf(CodeInternal, "listing the link intents of Target %d: %v", targetID, err)
	}
	for _, it := range intents {
		switch it.Action {
		case "create":
			if err := a.resolveCreateIntent(it); err != nil {
				return err
			}
		case "remove":
			if err := a.resolveRemoveIntent(it); err != nil {
				return err
			}
		default:
			return Errorf(CodeInternal, "link intent %d has unknown action %q", it.ID, it.Action)
		}
	}
	return nil
}
