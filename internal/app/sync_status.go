package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	"skillctl/internal/source"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// storeTreeState samples the live Store tree of one Skill: its canonical
// digest, or which way it is unusable. A missing tree is store_missing; a
// non-directory, unreadable, or invalid-Skill tree is store_invalid, so a
// plain file or a broken SKILL.md marker is never mistaken for content.
func (a *App) storeTreeState(ctx context.Context, slug string) (digest string, missing, invalid bool) {
	dir, err := a.store.SkillDir(slug)
	if err != nil {
		return "", false, true
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", true, false
	}
	if err != nil || !info.IsDir() {
		return "", false, true
	}
	d, err := source.TreeDigest(ctx, dir)
	if err != nil {
		return "", false, true
	}
	if err := source.ValidateSkillDir(dir); err != nil {
		return d, false, true
	}
	return d, false, false
}

// sourceEntryFacts locates one Binding's relative directory in a fresh
// observation: the valid entry, the validation issue, or absence.
func sourceEntryFacts(src *source.Source, relativeDir string) (entry *source.Entry, issue *source.Issue) {
	for i := range src.Entries {
		if src.Entries[i].RelativeDir == relativeDir {
			return &src.Entries[i], nil
		}
	}
	for i := range src.Issues {
		if src.Issues[i].RelativeDir == relativeDir {
			return nil, &src.Issues[i]
		}
	}
	return nil, nil
}

// evaluateSkillSync computes one Skill's relationship from a fresh
// observation and the live Store tree. An unavailable Source retains the
// previous relationship and marks it stale; a failed observation never
// invents a missing entry.
func (a *App) evaluateSkillSync(ctx context.Context, d state.SkillDetail, src *source.Source) (status sync.Status, stale bool, srcDigest, storeDigest string, err error) {
	if d.Binding == nil {
		return sync.StatusUnbound, false, "", "", nil
	}
	storeDigest, missing, invalid := a.storeTreeState(ctx, d.Skill.Slug)
	entry, issue := sourceEntryFacts(src, d.Binding.RelativeDir)
	in := sync.StatusInput{
		Bound:         true,
		SourceAvail:   src.Available,
		SourceMissing: src.Available && entry == nil && issue == nil,
		SourceInvalid: src.Available && issue != nil,
		StoreMissing:  missing,
		StoreInvalid:  invalid,
		StoreDigest:   storeDigest,
		Baseline:      d.Skill.BaselineDigest,
	}
	if entry != nil {
		in.SourceDigest = entry.Digest
	}
	status, stale = sync.Evaluate(sync.Status(d.Skill.SyncStatus), in)
	return status, stale, in.SourceDigest, storeDigest, nil
}

// persistSyncStatus writes one Skill's evaluated relationship.
func (a *App) persistSyncStatus(skillID int64, status sync.Status, stale bool) error {
	now := time.Now().UTC()
	if err := state.UpdateSyncStatus(a.db, skillID, string(status), stale, now); err != nil {
		return Errorf(CodeInternal, "recording the Sync Status of Skill %d: %v", skillID, err)
	}
	return nil
}

// CheckSkillSync re-observes one Skill's Source and persists the evaluated
// relationship. It never rewrites live Skill content; the only write beyond
// the comparison state is the decision-05 Baseline advancement when the
// Source and the Store converged independently on content the persisted
// Baseline does not yet carry. The Skill facts, the persisted Source facts,
// and the live tree are all re-sampled under the Store exclusive lock, so
// the convergence decision and the in_sync marking never use a stale
// outside sample (TOCTOU-free), and the advancement runs through the
// durable Baseline journal.
func (a *App) CheckSkillSync(ctx context.Context, skillID int64) (*Skill, error) {
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", skillID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
	}
	if d.Binding == nil {
		if err := a.persistSyncStatus(skillID, sync.StatusUnbound, false); err != nil {
			return nil, err
		}
		d.Skill.SyncStatus = string(sync.StatusUnbound)
		s := appSkill(*d)
		return &s, nil
	}
	if _, err := a.observeSource(ctx, d.Binding.SourceID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := a.withStoreLock(ctx, func() error {
		status, stale, err := a.convergeIfConfirmedLocked(ctx, skillID)
		if err != nil {
			return err
		}
		if err := a.persistSyncStatus(skillID, status, stale); err != nil {
			return err
		}
		d, err = state.GetSkillDetailByID(a.db, skillID)
		if err != nil {
			return err
		}
		d.Skill.SyncStatus = string(status)
		d.Skill.SyncStale = stale
		d.Skill.SyncCheckedAt = &now
		return nil
	}); err != nil {
		return nil, err
	}
	s := appSkill(*d)
	return &s, nil
}

// withStoreLock runs fn under the Store exclusive lock with all open
// operations recovered; it exists so every synchronization write path
// shares one lock-and-recover sequence.
func (a *App) withStoreLock(ctx context.Context, fn func() error) error {
	held, err := a.acquireStoreLock()
	if err != nil {
		return err
	}
	defer held.Unlock()
	if err := a.recoverOpenOperations(context.WithoutCancel(ctx)); err != nil {
		return err
	}
	return fn()
}
