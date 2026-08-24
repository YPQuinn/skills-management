package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/lock"
	"skillctl/internal/state"
)

// acquireStoreSharedLock takes the Store shared lock. Distribution and
// inspection hold it for their whole Target visit, so a Store replacement
// never swaps a link destination mid-observation; Store writes hold the
// exclusive lock (decision 08).
func (a *App) acquireStoreSharedLock() (*lock.Lock, error) {
	path, err := a.storeLockPath()
	if err != nil {
		return nil, Errorf(CodeInternal, "preparing Store lock: %v", err)
	}
	held, err := lock.TryShared(path)
	if errors.Is(err, lock.ErrLocked) {
		return nil, Errorf(CodeLocked, "another skillctl process is writing the Skill Store")
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "locking the Skill Store: %v", err)
	}
	return held, nil
}

// targetLockPath is the per-Target exclusive lock, in Skill Manager state
// rather than in the Target (decision 06).
func (a *App) targetLockPath(targetID int64) (string, error) {
	dir := filepath.Join(filepath.Dir(a.StateDBPath), "locks", "targets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%d.lock", targetID)), nil
}

// acquireTargetLock takes one Target's exclusive cross-process lock. The
// caller must hold the Store shared lock first; multi-Target operations
// acquire Target locks in stable Target-ID order (decision 06).
func (a *App) acquireTargetLock(targetID int64) (*lock.Lock, error) {
	path, err := a.targetLockPath(targetID)
	if err != nil {
		return nil, Errorf(CodeInternal, "preparing Target lock: %v", err)
	}
	held, err := lock.TryExclusive(path)
	if errors.Is(err, lock.ErrLocked) {
		return nil, Errorf(CodeLocked, "another skillctl process is reconciling this Target")
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "locking the Target: %v", err)
	}
	return held, nil
}

// InspectTarget runs one fresh, coherent Target inspection: the safety
// gate, then the observation of exactly the desired slugs and the Managed
// Link ledger paths. The observation is persisted and returned; an
// incomplete inspection retains the previous observation and marks it
// stale (decision 06).
func (a *App) InspectTarget(ctx context.Context, targetID int64) (*DistributionStatus, error) {
	t, err := state.GetTargetByID(a.db, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", targetID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", targetID, err)
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
	st, err := a.inspectTarget(ctx, t)
	if err != nil {
		return nil, err
	}
	return st.status, nil
}

// inspectTarget runs the gate and one coherent inspection for one Target
// whose caller holds the Store shared lock and this Target's exclusive
// lock. Every observation is persisted before it is returned, and gate
// or inspection failures retain the previous observation marked stale.
func (a *App) inspectTarget(ctx context.Context, t *state.Target) (*inspectedTarget, error) {
	opts, err := resolveOptions()
	if err != nil {
		return nil, err
	}
	storeRoot, err := a.storeLinkRoot()
	if err != nil {
		return nil, err
	}
	gate := distribution.GateTarget(t.Path, a.StorePath, opts)
	out := &inspectedTarget{gate: gate, storeRoot: storeRoot,
		status: &DistributionStatus{TargetID: t.ID, Name: t.Name, Path: t.Path,
			State: gate.State, StateError: gate.Error, Items: []DistributionItemView{}}}

	if gate.State == distribution.StateRedirected || gate.State == distribution.StateInvalid {
		if err := a.markInspectionStale(t.ID, gate.Error); err != nil {
			return nil, err
		}
		fresh, err := state.GetTargetByID(a.db, t.ID)
		if err != nil {
			return nil, Errorf(CodeInternal, "re-reading Target %d: %v", t.ID, err)
		}
		if err := a.fillStoredStatus(out, fresh); err != nil {
			return nil, err
		}
		return out, nil
	}

	desired, err := state.ExpandDesiredSet(a.db, t.ID)
	if err != nil {
		return nil, Errorf(CodeInternal, "expanding the desired set of Target %d: %v", t.ID, err)
	}
	ledger, err := state.ListManagedLinksByTarget(a.db, t.ID)
	if err != nil {
		return nil, Errorf(CodeInternal, "reading the Managed Link ledger of Target %d: %v", t.ID, err)
	}

	desiredSlugs := map[string]int64{}
	for _, d := range desired {
		desiredSlugs[d.Skill.Slug] = d.Skill.ID
	}
	claims := map[string]state.ManagedLinkByTargetSkill{}
	for _, l := range ledger {
		claims[l.Slug] = l
	}
	relations := make([]distribution.Relation, 0, len(desired)+len(ledger))
	for _, d := range desired {
		// A desired slug keeps its ledger claim: a matching symlink can
		// only be proven managed through its recorded identity and raw
		// target.
		rel := distribution.Relation{Slug: d.Skill.Slug}
		if c, ok := claims[d.Skill.Slug]; ok {
			rel.LedgerRaw, rel.LedgerDev, rel.LedgerIno, rel.LedgerMtime = c.RawTarget, c.LinkDev, c.LinkIno, c.LinkMtime
		}
		relations = append(relations, rel)
	}
	for _, l := range ledger {
		if _, ok := desiredSlugs[l.Slug]; ok {
			continue // already inspected once, with the claim
		}
		relations = append(relations, distribution.Relation{
			Slug: l.Slug, LedgerRaw: l.RawTarget, LedgerDev: l.LinkDev, LedgerIno: l.LinkIno, LedgerMtime: l.LinkMtime,
		})
		desiredSlugs[l.Slug] = l.SkillID
	}
	sort.Slice(relations, func(i, j int) bool { return relations[i].Slug < relations[j].Slug })

	var entries []distribution.Entry
	if gate.State == distribution.StateMissing {
		for _, rel := range relations {
			entries = append(entries, distribution.Entry{Slug: rel.Slug, Observed: distribution.ObservedMissing})
		}
	} else {
		entries, err = distribution.Inspect(gate.Path, relations, storeRoot)
		if err != nil {
			if err := a.markInspectionStale(t.ID, err.Error()); err != nil {
				return nil, err
			}
			fresh, err := state.GetTargetByID(a.db, t.ID)
			if err != nil {
				return nil, Errorf(CodeInternal, "re-reading Target %d: %v", t.ID, err)
			}
			if err := a.fillStoredStatus(out, fresh); err != nil {
				return nil, err
			}
			return out, nil
		}
	}

	now := time.Now().UTC()
	prev, err := state.ListDistributionItems(a.db, t.ID)
	if err != nil {
		return nil, Errorf(CodeInternal, "reading the stored observation of Target %d: %v", t.ID, err)
	}
	prevOutcome := map[int64]state.DistributionItem{}
	for _, p := range prev {
		prevOutcome[p.SkillID] = p
	}

	items := make([]state.DistributionItem, 0, len(entries))
	for _, e := range entries {
		skillID := desiredSlugs[e.Slug]
		desiredState := distribution.DesiredAbsent
		if _, ok := desiredBySlug(desired, e.Slug); ok {
			desiredState = distribution.DesiredPresent
		}
		it := state.DistributionItem{
			TargetID: t.ID, SkillID: skillID, Slug: e.Slug,
			Desired: desiredState, Observed: e.Observed,
			Managed: e.Managed, Adoptable: e.Adoptable,
			NodeKind: e.NodeKind, RawTarget: e.RawTarget, ResolvedTarget: e.ResolvedTarget,
			InspectedAt: &now,
		}
		if p, ok := prevOutcome[skillID]; ok {
			it.LastResult, it.LastError = p.LastResult, p.LastError
		}
		items = append(items, it)
		out.plan = append(out.plan, a.planFacts(it, storeRoot))
	}
	if err := state.ReplaceDistributionItems(a.db, t.ID, items); err != nil {
		return nil, Errorf(CodeInternal, "persisting the observation of Target %d: %v", t.ID, err)
	}
	if err := state.UpdateTargetInspection(a.db, t.ID, state.TargetDistribution{InspectedAt: &now}); err != nil {
		return nil, Errorf(CodeInternal, "recording the inspection of Target %d: %v", t.ID, err)
	}
	out.status.InspectedAt = &now
	out.status.Items = itemViews(items, storeRoot)
	return out, nil
}

// planFacts fills one relation's plan facts from its observation.
func (a *App) planFacts(it state.DistributionItem, storeRoot string) planItem {
	p := planItem{
		skillID: it.SkillID, slug: it.Slug, desired: it.Desired, observed: it.Observed,
		rawTarget: distribution.ExpectedPath(storeRoot, it.Slug),
	}
	if it.Desired == distribution.DesiredPresent {
		p.storeOK, p.storeError = storeSkillOK(storeRoot, it.Slug)
	}
	return p
}

// markInspectionStale retains the previous observation, records the
// failure, and marks it stale (decision 06).
func (a *App) markInspectionStale(targetID int64, errMsg string) error {
	if err := state.MarkDistributionItemsStale(a.db, targetID, errMsg); err != nil {
		return Errorf(CodeInternal, "marking the observation of Target %d stale: %v", targetID, err)
	}
	now := time.Now().UTC()
	if err := state.UpdateTargetInspection(a.db, targetID, state.TargetDistribution{
		InspectedAt: &now, InspectedStale: true, InspectError: errMsg,
	}); err != nil {
		return Errorf(CodeInternal, "recording the failed inspection of Target %d: %v", targetID, err)
	}
	return nil
}
