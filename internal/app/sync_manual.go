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

// manualSyncInput is the shared prelude of the explicit synchronization
// actions: the resolved Skill detail and one fresh Source observation.
func (a *App) manualSyncInput(ctx context.Context, skillID int64) (*state.SkillDetail, *source.Source, error) {
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, Errorf(CodeNotFound, "Skill %d not found", skillID)
	}
	if err != nil {
		return nil, nil, Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
	}
	if d.Binding == nil {
		return nil, nil, Errorf(CodeInvalidArgument, "Skill %q is not bound to a Source", d.Skill.Slug)
	}
	src, err := a.observeSource(ctx, d.Binding.SourceID)
	if err != nil {
		return nil, nil, err
	}
	return d, src, nil
}

// manualOutcomeOf finishes one explicit-action outcome: the relationship
// is evaluated from the fresh observation (retaining the previous status
// marked stale when the Source is unreachable) and both the status and the
// latest outcome are persisted, so blocked and no-op paths never leave an
// empty item status.
func (a *App) manualOutcomeOf(ctx context.Context, d *state.SkillDetail, src *source.Source, item SyncItemResult, started time.Time) *SyncItemResult {
	if status, stale, _, _, err := a.evaluateSkillSync(ctx, *d, src); err == nil {
		item.Status, item.Stale = status, stale
		if err := a.persistSyncStatus(d.Skill.ID, status, stale); err != nil {
			item.ErrorCode = CodeInternal
		}
	} else {
		item.ErrorCode = CodeInternal
	}
	if err := a.persistSyncOutcome(d.Skill.ID, item, started); err != nil {
		item.ErrorCode = CodeInternal
	}
	return &item
}

// blockedOutcomeOf builds and persists a state-based blocked outcome.
func (a *App) blockedOutcomeOf(ctx context.Context, d *state.SkillDetail, src *source.Source, action, message string) *SyncItemResult {
	item := SyncItemResult{
		SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: action,
		Result: sync.ResultBlocked, ErrorMessage: message,
		BeforeDigest: d.Skill.StoreDigest, Revision: src.LastCommit,
	}
	return a.manualOutcomeOf(ctx, d, src, item, time.Now().UTC())
}

// noOpOutcomeOf builds and persists a completed no-op outcome.
func (a *App) noOpOutcomeOf(ctx context.Context, d *state.SkillDetail, src *source.Source, action string) *SyncItemResult {
	item := SyncItemResult{
		SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: action,
		Result: sync.ResultNoOp, BeforeDigest: d.Skill.StoreDigest,
		AfterDigest: d.Skill.StoreDigest, Revision: src.LastCommit,
	}
	return a.manualOutcomeOf(ctx, d, src, item, time.Now().UTC())
}

// KeepStore leaves live content untouched and accepts the currently
// observed Source content as the new Synchronization Baseline. A conflict
// consequently becomes store_changed; a later Source change becomes a new
// conflict.
func (a *App) KeepStore(ctx context.Context, skillID int64) (*SyncItemResult, error) {
	d, src, err := a.manualSyncInput(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if !src.Available {
		return a.blockedOutcomeOf(ctx, d, src, sync.ActionKeepStore,
			"Source "+src.Name+" is not reachable: "+src.LastError), nil
	}
	entry, issue := sourceEntryFacts(src, d.Binding.RelativeDir)
	if entry == nil {
		if issue != nil {
			return a.blockedOutcomeOf(ctx, d, src, sync.ActionKeepStore,
				"the Source entry failed validation: "+issue.Reason), nil
		}
		return a.blockedOutcomeOf(ctx, d, src, sync.ActionKeepStore,
			"the Source entry is no longer in the Inventory"), nil
	}
	if entry.Digest == d.Skill.BaselineDigest {
		return a.noOpOutcomeOf(ctx, d, src, sync.ActionKeepStore), nil
	}
	started := time.Now().UTC()
	item := SyncItemResult{SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: sync.ActionKeepStore,
		BeforeDigest: d.Skill.StoreDigest, AfterDigest: d.Skill.StoreDigest, Revision: src.LastCommit}
	if err := a.withStoreLock(ctx, func() error {
		fresh, err := state.GetSkillDetailByID(a.db, skillID)
		if err != nil {
			return Errorf(CodeInternal, "reading Skill %d under the Store lock: %v", skillID, err)
		}
		if fresh.Binding == nil {
			return Errorf(CodeConflict, "Skill %q is no longer bound to a Source", fresh.Skill.Slug)
		}
		if fresh.Binding.SourceID != src.ID {
			return Errorf(CodeConflict, "Skill %q is now bound to a different Source", fresh.Skill.Slug)
		}
		// The entry is re-resolved from the fresh Binding in the fresh
		// observation: a Binding re-pointed at another entry of the same
		// Source must never act on the stale outside entry.
		entry, issue = sourceEntryFacts(src, fresh.Binding.RelativeDir)
		if entry == nil {
			if issue != nil {
				item = *a.blockedOutcomeOf(ctx, fresh, src, sync.ActionKeepStore,
					"the Source entry failed validation: "+issue.Reason)
			} else {
				item = *a.blockedOutcomeOf(ctx, fresh, src, sync.ActionKeepStore,
					"the Source entry is no longer in the Inventory")
			}
			return nil
		}
		if fresh.Skill.BaselineDigest == entry.Digest {
			// A concurrent acceptance already advanced the Baseline.
			item.Result = sync.ResultNoOp
			item.Status = sync.RecheckStatus(sync.StatusInput{
				Bound: true, SourceDigest: entry.Digest,
				StoreDigest: fresh.Skill.StoreDigest, Baseline: fresh.Skill.BaselineDigest,
			})
			item.Stale = !src.Available
			return nil
		}
		dir, digest, err := a.materializeSyncContent(ctx, src, *entry, true)
		if err != nil {
			item.Result, item.ErrorMessage = sync.ResultFailed, err.Error()
			item.ErrorCode = CodeSyncFailed
			if ctx.Err() != nil {
				item.ErrorCode = CodeCancelled
			}
			return nil
		}
		defer os.RemoveAll(dir)
		if fail := a.baselineRefresh(ctx, skillID, src, dir, digest); fail.code != "" {
			item.Result = sync.ResultFailed
			item.ErrorCode, item.ErrorMessage = fail.code, fail.message
			return nil
		}
		item.Result = sync.ResultKeptStore
		item.Status = sync.RecheckStatus(sync.StatusInput{
			Bound: true, SourceDigest: digest,
			StoreDigest: fresh.Skill.StoreDigest, Baseline: digest,
		})
		item.Stale = !src.Available
		if err := a.persistSyncStatus(skillID, item.Status, item.Stale); err != nil {
			item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := a.persistSyncOutcome(d.Skill.ID, item, started); err != nil {
		item.ErrorCode = CodeInternal
	}
	return &item, nil
}

// AcceptSource snapshots the current Store content, atomically replaces it
// with validated Source content, and advances the Baseline to in_sync. It
// is the only repair for store_missing and store_invalid.
func (a *App) AcceptSource(ctx context.Context, skillID int64) (*SyncItemResult, error) {
	d, src, err := a.manualSyncInput(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if !src.Available {
		return a.blockedOutcomeOf(ctx, d, src, sync.ActionAcceptSource,
			"Source "+src.Name+" is not reachable: "+src.LastError), nil
	}
	entry, issue := sourceEntryFacts(src, d.Binding.RelativeDir)
	if entry == nil {
		if issue != nil {
			return a.blockedOutcomeOf(ctx, d, src, sync.ActionAcceptSource,
				"the Source entry failed validation: "+issue.Reason), nil
		}
		return a.blockedOutcomeOf(ctx, d, src, sync.ActionAcceptSource,
			"the Source entry is no longer in the Inventory"), nil
	}
	started := time.Now().UTC()
	item := SyncItemResult{SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: sync.ActionAcceptSource,
		BeforeDigest: d.Skill.StoreDigest, Revision: src.LastCommit}
	if err := a.withStoreLock(ctx, func() error {
		// The Skill facts are re-read under the held lock and the Binding
		// is verified against the fresh observation before any commit.
		fresh, err := state.GetSkillDetailByID(a.db, skillID)
		if err != nil {
			return Errorf(CodeInternal, "reading Skill %d under the Store lock: %v", skillID, err)
		}
		if fresh.Binding == nil {
			return Errorf(CodeConflict, "Skill %q is no longer bound to a Source", fresh.Skill.Slug)
		}
		if fresh.Binding.SourceID != src.ID {
			return Errorf(CodeConflict, "Skill %q is now bound to a different Source", fresh.Skill.Slug)
		}
		d = fresh
		// The entry is re-resolved from the fresh Binding in the fresh
		// observation, so an acceptance never acts on a stale entry.
		entry, issue = sourceEntryFacts(src, d.Binding.RelativeDir)
		if entry == nil {
			if issue != nil {
				item = *a.blockedOutcomeOf(ctx, d, src, sync.ActionAcceptSource,
					"the Source entry failed validation: "+issue.Reason)
			} else {
				item = *a.blockedOutcomeOf(ctx, d, src, sync.ActionAcceptSource,
					"the Source entry is no longer in the Inventory")
			}
			return nil
		}
		storeDigest, missing, invalid := a.storeTreeState(ctx, d.Skill.Slug)
		item.BeforeDigest = storeDigest
		if !missing && !invalid && storeDigest == entry.Digest {
			if d.Skill.BaselineDigest != entry.Digest {
				status, stale, err := a.convergeIfConfirmedLocked(ctx, d.Skill.ID)
				if err != nil {
					item.Result = sync.ResultFailed
					item.ErrorCode, item.ErrorMessage = errorCodeOf(err), err.Error()
					return nil
				}
				item.Status, item.Stale = status, stale
			} else {
				item.Status, item.Stale = sync.StatusInSync, false
			}
			item.Result = sync.ResultNoOp
			item.AfterDigest = storeDigest
			if err := a.persistSyncStatus(d.Skill.ID, item.Status, item.Stale); err != nil {
				item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
			}
			return nil
		}
		dir, digest, err := a.materializeSyncContent(ctx, src, *entry, false)
		if err != nil {
			item.Result, item.ErrorMessage = sync.ResultFailed, err.Error()
			item.ErrorCode = CodeSyncFailed
			if ctx.Err() != nil {
				item.ErrorCode = CodeCancelled
			}
			return nil
		}
		defer os.RemoveAll(dir)
		mode := modeReplaceAdvance
		if missing {
			mode = modeRepairImport
		}
		// The explicit acceptance replaces the live tree the confirmation
		// sampled under the lock (storeDigest), not an earlier snapshot:
		// a local edit landing during the materialization is refused as a
		// conflict and a retry re-confirms the current live.
		if fail := a.replaceSkillContent(ctx, d, src, entry, dir, digest, mode, "accept_source", storeDigest); fail.code != "" {
			item.Result = sync.ResultFailed
			item.ErrorCode, item.ErrorMessage = fail.code, fail.message
			if fail.committed {
				item.AfterDigest = digest
			}
			return nil
		}
		item.Result = sync.ResultAcceptedSource
		item.AfterDigest = digest
		item.Status, item.Stale = sync.StatusInSync, false
		if err := a.persistSyncStatus(d.Skill.ID, sync.StatusInSync, false); err != nil {
			item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := a.persistSyncOutcome(d.Skill.ID, item, started); err != nil {
		item.ErrorCode = CodeInternal
	}
	return &item, nil
}
