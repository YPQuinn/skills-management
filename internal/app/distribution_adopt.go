package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// AdoptResult is the outcome of one explicit adoption.
type AdoptResult struct {
	TargetID  int64     `json:"target_id"`
	SkillID   int64     `json:"skill_id"`
	Slug      string    `json:"slug"`
	LinkPath  string    `json:"link_path"`
	RawTarget string    `json:"raw_target"`
	AdoptedAt time.Time `json:"adopted_at"`
	Result    string    `json:"result"`
}

// AdoptTargetLink explicitly adopts one eligible existing symlink as a
// Managed Link. Adoption rechecks the path, records its existing raw
// target, and does not rewrite the link; it succeeds only when a fresh
// physical resolution proves the link points exactly at the currently
// desired, valid Store Skill (decision 06). Files, directories,
// wrong-target links, and links outside the Store cannot be adopted, and
// there is no automatic adoption.
func (a *App) AdoptTargetLink(ctx context.Context, targetID, skillID int64) (*AdoptResult, error) {
	t, err := state.GetTargetByID(a.db, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", targetID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", targetID, err)
	}
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", skillID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
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

	opts, err := resolveOptions()
	if err != nil {
		return nil, err
	}
	gate := distribution.GateTarget(t.Path, a.StorePath, opts)
	switch gate.State {
	case distribution.StateRedirected, distribution.StateInvalid:
		return nil, Errorf(CodeTargetConflict, "the Target cannot be adopted into: %s", gate.Error)
	case distribution.StateMissing:
		return nil, Errorf(CodeTargetConflict, "the Target container is missing")
	}

	desired, err := state.ExpandDesiredSet(a.db, t.ID)
	if err != nil {
		return nil, Errorf(CodeInternal, "expanding the desired set of Target %d: %v", t.ID, err)
	}
	found := false
	for _, ds := range desired {
		if ds.Skill.ID == skillID {
			found = true
			break
		}
	}
	if !found {
		return nil, Errorf(CodeTargetConflict, "Skill %q is not currently desired at Target %q", d.Skill.Slug, t.Name)
	}

	storeRoot, err := a.storeLinkRoot()
	if err != nil {
		return nil, err
	}
	expected := distribution.ExpectedPath(storeRoot, d.Skill.Slug)
	if ok, reason := storeSkillOK(storeRoot, d.Skill.Slug); !ok {
		return nil, Errorf(CodeTargetConflict, "the link cannot be adopted: %s", reason)
	}
	raw, err := distribution.ProbeAdoption(gate.Path, d.Skill.Slug, expected)
	if err != nil {
		return nil, Errorf(CodeTargetConflict, "the link cannot be adopted: %v", err)
	}

	now := time.Now().UTC()
	linkPath := filepath.Join(t.Path, d.Skill.Slug)
	if err := state.ReplaceManagedLink(a.db, state.ManagedLink{
		TargetID: t.ID, SkillID: skillID, LinkPath: linkPath,
		RawTarget: raw, EstablishedAt: now,
	}); err != nil {
		return nil, Errorf(CodeInternal, "recording the adopted link: %v", err)
	}
	if result, msg := a.recordItemOutcome(t, planItem{
		skillID: skillID, slug: d.Skill.Slug, desired: distribution.DesiredPresent, observed: distribution.ObservedLinked,
	}, distribution.OutcomeAdopted, distribution.ObservedLinked, "", now); result != distribution.OutcomeAdopted {
		return nil, Errorf(CodeInternal, "recording the adoption: %s", msg)
	}
	if _, err := a.inspectTarget(ctx, t); err != nil {
		return nil, err
	}
	return &AdoptResult{
		TargetID: t.ID, SkillID: skillID, Slug: d.Skill.Slug,
		LinkPath: linkPath, RawTarget: raw, AdoptedAt: now,
		Result: distribution.OutcomeAdopted,
	}, nil
}
