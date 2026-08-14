package app

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// DiffResult is the three-way path difference of one bound Skill
// (decision 05): the three comparisons between the Baseline, the Source,
// and the Store, with per-path change entries.
type DiffResult struct {
	SkillID        int64             `json:"skill_id"`
	Slug           string            `json:"slug"`
	SourceDigest   string            `json:"source_digest"`
	StoreDigest    string            `json:"store_digest"`
	BaselineDigest string            `json:"baseline_digest"`
	Comparisons    []sync.Comparison `json:"comparisons"`
}

// DiffSkill computes the three-way difference of one bound Skill. The
// Source side comes from a fresh observation materialized as the effective
// tree; the Store and Baseline sides are walked from their live trees. An
// unavailable, missing, or invalid Source entry blocks the diff, because
// the decision-05 comparisons all involve the Source. path filters every
// comparison to one path and its descendants.
func (a *App) DiffSkill(ctx context.Context, skillID int64, path string) (*DiffResult, error) {
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", skillID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
	}
	if d.Binding == nil {
		return nil, Errorf(CodeInvalidArgument, "Skill %q is not bound to a Source", d.Skill.Slug)
	}
	src, err := a.observeSource(ctx, d.Binding.SourceID)
	if err != nil {
		return nil, err
	}
	if !src.Available {
		return nil, Errorf(CodeSourceUnavailable, "Source %s is not reachable: %s", src.Name, src.LastError)
	}
	entry, issue := sourceEntryFacts(src, d.Binding.RelativeDir)
	if entry == nil {
		if issue != nil {
			return nil, Errorf(CodeConflict, "the Source entry failed validation: %s", issue.Reason)
		}
		return nil, Errorf(CodeConflict, "the Source entry is no longer in the Inventory")
	}
	dir, digest, err := a.materializeSyncContent(ctx, src, *entry, true)
	if err != nil {
		return nil, Errorf(CodeSyncFailed, "materializing the Source content: %v", err)
	}
	defer os.RemoveAll(dir)
	sourceTree, err := sync.SnapshotTree(ctx, dir)
	if err != nil {
		return nil, Errorf(CodeSyncFailed, "snapshotting the Source content: %v", err)
	}
	storeTree, storeDigest, err := a.snapshotStoreTree(ctx, d.Skill.Slug)
	if err != nil {
		return nil, err
	}
	baselineTree, err := a.snapshotBaselineTree(ctx, d.Skill.ID)
	if err != nil {
		return nil, err
	}
	result := &DiffResult{
		SkillID: skillID, Slug: d.Skill.Slug,
		SourceDigest: digest, StoreDigest: storeDigest,
		BaselineDigest: d.Skill.BaselineDigest,
	}
	for _, c := range sync.ThreeWay(baselineTree, sourceTree, storeTree) {
		result.Comparisons = append(result.Comparisons, c.FilterEntries(path))
	}
	return result, nil
}

// snapshotStoreTree walks the live Store tree and returns its current
// canonical digest; a missing tree is an empty snapshot with an empty
// digest so the comparisons still show its absence honestly.
func (a *App) snapshotStoreTree(ctx context.Context, slug string) (sync.Tree, string, error) {
	dir, err := a.store.SkillDir(slug)
	if err != nil {
		return nil, "", Errorf(CodeInternal, "resolving the Store path: %v", err)
	}
	digest, missing, invalid := a.storeTreeState(ctx, slug)
	t, err := sync.SnapshotTree(ctx, dir)
	if errors.Is(err, os.ErrNotExist) {
		return sync.Tree{}, "", nil
	}
	if err != nil {
		return nil, "", Errorf(CodeInternal, "reading the Store content: %v", err)
	}
	if missing || invalid {
		return t, digest, nil
	}
	return t, digest, nil
}

// snapshotBaselineTree walks the internal Baseline tree; a missing tree is
// an empty snapshot.
func (a *App) snapshotBaselineTree(ctx context.Context, skillID int64) (sync.Tree, error) {
	dir := a.store.BaselineDir(skillID)
	t, err := sync.SnapshotTree(ctx, dir)
	if errors.Is(err, os.ErrNotExist) {
		return sync.Tree{}, nil
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading the Baseline content: %v", err)
	}
	return t, nil
}
