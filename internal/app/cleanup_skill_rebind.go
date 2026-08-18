package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// RebindSkill binds one Skill to an explicit Source Inventory entry.
// Identical content becomes in_sync with a new Baseline; different content
// enters conflict without overwriting the Store. Identical is taken from
// the final Sync Status and live/Source digest match, not the pre-converge
// sample.
func (a *App) RebindSkill(ctx context.Context, skillID, sourceID int64, relativeDir, name string) (*RebindResult, error) {
	if relativeDir != "" && name != "" {
		return nil, Errorf(CodeInvalidArgument, "choose either a relative path or a name, not both")
	}
	if relativeDir == "" && name == "" {
		return nil, Errorf(CodeInvalidArgument, "a relative path or a name is required")
	}
	if _, err := state.GetSkillDetailByID(a.db, skillID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", skillID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
	}
	src, err := a.observeSource(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if !src.Available {
		return nil, Errorf(CodeSourceUnavailable, "Source %s is not reachable: %s", src.Name, src.LastError)
	}
	entry, err := resolveRebindEntry(src.Entries, relativeDir, name)
	if err != nil {
		return nil, err
	}
	if existing, err := state.GetSkillBySourceEntry(a.db, sourceID, entry.RelativeDir); err == nil && existing.Skill.ID != skillID {
		return nil, Errorf(CodeConflict, "Source entry %q is already bound to Skill %q", entry.RelativeDir, existing.Skill.Slug)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeInternal, "checking the existing Binding: %v", err)
	}

	if err := a.withStoreLock(ctx, func() error {
		return a.rebindSkillLocked(ctx, skillID, sourceID, src, entry)
	}); err != nil {
		return nil, err
	}
	sk, err := a.ShowSkill(skillID)
	if err != nil {
		return nil, err
	}
	live, missing, invalid := a.storeTreeState(ctx, sk.Slug)
	srcDigest := ""
	if sk.Binding != nil {
		srcDigest = sk.Binding.Digest
	}
	identical := sk.SyncStatus == string(sync.StatusInSync) && !missing && !invalid && srcDigest != "" && srcDigest == live
	return &RebindResult{Skill: *sk, Identical: identical}, nil
}

func (a *App) rebindSkillLocked(ctx context.Context, skillID, sourceID int64, src *source.Source, entry *source.Entry) error {
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		return Errorf(CodeInternal, "re-reading Skill %d: %v", skillID, err)
	}
	now := time.Now().UTC()
	prev := snapshotRebind(d)
	live, missing, invalid := a.storeTreeState(ctx, d.Skill.Slug)
	matches := !missing && !invalid && entry.Digest != "" && entry.Digest == live
	if !matches {
		return a.clearBaselineJournaled(d, prev, sourceID, src, entry, now)
	}
	if err := state.UpsertBinding(a.db, state.Binding{
		SkillID: skillID, SourceID: sourceID, RelativeDir: entry.RelativeDir,
		Digest: entry.Digest, SourceCommit: src.LastCommit, ImportedAt: now,
	}); err != nil {
		if state.IsUniqueViolation(err) {
			return Errorf(CodeConflict, "Source entry %q is already bound to another Skill", entry.RelativeDir)
		}
		return Errorf(CodeInternal, "rebinding Skill %q: %v", d.Skill.Slug, err)
	}
	fail := func(err error) error {
		if a.rebindHasCommittedJournal(skillID) {
			return err
		}
		if rerr := a.restoreRebind(skillID, prev); rerr != nil {
			return Errorf(CodeInternal, "rebinding Skill %q failed (%v) and the previous Binding could not be restored: %v", d.Skill.Slug, err, rerr)
		}
		return err
	}
	status, stale, err := a.convergeIfConfirmedLocked(ctx, skillID)
	if err != nil {
		return fail(err)
	}
	return a.persistSyncStatus(skillID, status, stale)
}

func (a *App) clearBaselineJournaled(d *state.SkillDetail, prev rebindSnapshot, sourceID int64, src *source.Source, entry *source.Entry, now time.Time) error {
	b := state.Binding{
		SkillID: d.Skill.ID, SourceID: sourceID, RelativeDir: entry.RelativeDir,
		Digest: entry.Digest, SourceCommit: src.LastCommit, ImportedAt: now,
	}
	err := a.runBaselineClear(d, func(op skillstore.Operation) error {
		if err := a.commitClear(op, d.Skill.ID, d.Skill.BaselineDigest, &b, string(sync.StatusConflict)); err != nil {
			return Errorf(CodeInternal, "committing the conflicting Rebind of Skill %q: %v", d.Skill.Slug, err)
		}
		return nil
	})
	if err != nil && !a.rebindHasCommittedJournal(d.Skill.ID) {
		if rerr := a.restoreRebind(d.Skill.ID, prev); rerr != nil {
			return Errorf(CodeInternal, "rebinding Skill %q failed (%v) and the previous Binding could not be restored: %v", d.Skill.Slug, err, rerr)
		}
	}
	return err
}

// rebindHasCommittedJournal reports a Baseline journal that already
// committed (or reached a terminal receipt). Rolling the Binding back
// would leave that journal paired with the pre-Rebind DB row.
func (a *App) rebindHasCommittedJournal(skillID int64) bool {
	ops, err := state.ListOpenOperations(a.db)
	if err != nil {
		return true
	}
	for _, op := range ops {
		if op.SkillID != skillID {
			continue
		}
		switch op.Phase {
		case skillstore.PhaseCommitted, skillstore.PhaseFinalized, skillstore.PhaseRestored:
			return true
		}
	}
	return false
}

type rebindSnapshot struct {
	binding        *state.Binding
	baselineDigest string
	syncStatus     string
	syncStale      bool
}

func snapshotRebind(d *state.SkillDetail) rebindSnapshot {
	s := rebindSnapshot{
		baselineDigest: d.Skill.BaselineDigest,
		syncStatus:     d.Skill.SyncStatus,
		syncStale:      d.Skill.SyncStale,
	}
	if d.Binding != nil {
		b := d.Binding.Binding
		s.binding = &b
	}
	return s
}

func (a *App) restoreRebind(skillID int64, prev rebindSnapshot) error {
	if prev.binding == nil {
		if err := state.DeleteBinding(a.db, skillID); err != nil {
			return err
		}
	} else if err := state.UpsertBinding(a.db, *prev.binding); err != nil {
		return err
	}
	if err := state.SetSkillBaselineDigest(a.db, skillID, prev.baselineDigest, time.Now().UTC()); err != nil {
		return err
	}
	return a.persistSyncStatus(skillID, sync.Status(prev.syncStatus), prev.syncStale)
}

func resolveRebindEntry(entries []source.Entry, relativeDir, name string) (*source.Entry, error) {
	if relativeDir != "" {
		for i := range entries {
			if entries[i].RelativeDir == relativeDir {
				return &entries[i], nil
			}
		}
		return nil, Errorf(CodeNotFound, "no Inventory entry at %q", relativeDir)
	}
	var matches []*source.Entry
	for i := range entries {
		if entries[i].Name == name {
			matches = append(matches, &entries[i])
		}
	}
	switch len(matches) {
	case 0:
		return nil, Errorf(CodeNotFound, "no Inventory entry named %q", name)
	case 1:
		return matches[0], nil
	default:
		return nil, Errorf(CodeInvalidArgument, "name %q is ambiguous; use its relative path", name)
	}
}
