package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"skillctl/internal/lock"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// observeSource performs exactly one fresh coherent Source observation under
// the per-Source cross-process lock: it re-reads the Source row, re-checks
// the Store-overlap guard, calls the observer once, and persists the whole
// observation. A successful check replaces the Inventory and check metadata;
// a failed check keeps the previous Inventory and records only availability
// plus latest check metadata, returning the Source with Available=false and
// LastCheckResult=failed. Lock contention is CodeLocked and mutates nothing;
// a cancelled context returns before persistence and records nothing.
// Callers that require a reachable Source (import) treat Available=false as
// an unavailability failure before any Store work.
func (a *App) observeSource(ctx context.Context, id int64) (*source.Source, error) {
	lockPath, err := a.sourceLockPath(id)
	if err != nil {
		return nil, Errorf(CodeInternal, "preparing Source %d lock: %v", id, err)
	}
	held, err := lock.TryExclusive(lockPath)
	if errors.Is(err, lock.ErrLocked) {
		return nil, Errorf(CodeLocked, "another skillctl process is already checking Source %d", id)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "locking Source %d: %v", id, err)
	}
	defer held.Unlock()

	cur, err := state.GetSource(a.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, Errorf(CodeNotFound, "Source %d not found", id)
		}
		return nil, Errorf(CodeInternal, "reading Source %d: %v", id, err)
	}

	// The Store-overlap guard is re-evaluated on every Local observation: a
	// post-registration symlink swap of the Source root into the Store must
	// never be scanned, and the Source's physical identity is verified by
	// ResolveLocalSubpath inside the guard.
	if cur.Kind == source.KindLocal {
		if err := a.checkLocalStoreOverlap(cur.Locator); err != nil {
			return nil, err
		}
	}

	started := time.Now().UTC()
	obs, err := a.observer.Observe(ctx, cur.Locator, a.workDir())
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	now := time.Now().UTC()
	if err != nil {
		if errors.Is(err, lock.ErrLocked) {
			return nil, Errorf(CodeLocked, "another skillctl process is already checking Source %d", id)
		}
		cur.UpdatedAt = now
		cur.Available = false
		cur.LastError = err.Error()
		cur.LastCheckStartedAt = &started
		cur.LastCheckedAt = &now
		cur.LastCheckResult = source.CheckResultFailed
		if err := state.UpdateSourceStatus(a.db, id, *cur); err != nil {
			return nil, Errorf(CodeInternal, "recording Source %d availability: %v", id, err)
		}
		return cur, nil
	}
	cur.UpdatedAt = now
	cur.Available = true
	cur.LastError = ""
	cur.LastCheckStartedAt = &started
	cur.LastCheckedAt = &now
	cur.LastCheckResult = source.CheckResultOK
	cur.LastSuccessfulCheckAt = &now
	cur.LastCommit = obs.Commit
	cur.LastInventoryDigest = obs.Digest
	cur.Entries = obs.Entries
	cur.Issues = obs.Issues
	if err := state.ReplaceSourceObservation(a.db, id, *cur); err != nil {
		return nil, Errorf(CodeInternal, "saving Source %d observation: %v", id, err)
	}
	return cur, nil
}
