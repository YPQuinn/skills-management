package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"skillctl/internal/source"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// SyncSkill runs the safe synchronization of one bound Skill: exactly one
// fresh Source check, then the decision-05 rules — source_changed is the
// only state updated automatically, everything else is reported as no_op,
// skipped, or blocked. The outcome is a result value, never a top-level
// error for a state-based block.
func (a *App) SyncSkill(ctx context.Context, skillID int64) (*SyncItemResult, error) {
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
	var out syncOutcome
	if err := a.withStoreLock(ctx, func() error {
		// Recovery ran first; the Skill facts are re-read under the held
		// lock so a Binding change while waiting for the lock can never be
		// acted on through the outside sample.
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
		out = a.actSync(ctx, src, *fresh)
		return nil
	}); err != nil {
		return nil, err
	}
	return &out.result, nil
}

// actSync runs one Skill's safe-sync decision under the held Store lock
// with recovery already resolved, for the already-fresh observation and the
// Skill facts re-read under the lock. The outcome is always persisted as
// the latest sync outcome.
func (a *App) actSync(ctx context.Context, src *source.Source, d state.SkillDetail) syncOutcome {
	item := SyncItemResult{SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: sync.ActionSync,
		BeforeDigest: d.Skill.StoreDigest, Revision: src.LastCommit}
	started := time.Now().UTC()
	blocked := false
	finish := func() syncOutcome {
		if err := a.persistSyncOutcome(d.Skill.ID, item, started); err != nil {
			item.Result = sync.ResultFailed
			item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
		}
		return syncOutcome{result: item, blocked: blocked}
	}
	status, stale, srcDigest, liveDigest, err := a.evaluateSkillSync(ctx, d, src)
	if err != nil {
		item.Result, item.ErrorCode, item.ErrorMessage = sync.ResultFailed, CodeInternal, "evaluating the Skill: "+err.Error()
		return finish()
	}
	item.Status, item.Stale = sync.Status(status), stale
	if err := a.persistSyncStatus(d.Skill.ID, status, stale); err != nil {
		item.Result, item.ErrorCode, item.ErrorMessage = sync.ResultFailed, CodeInternal, err.Error()
		return finish()
	}
	if !src.Available {
		item.Result = sync.ResultBlocked
		item.ErrorMessage = "Source " + src.Name + " is not reachable: " + src.LastError
		return finish()
	}
	item.BeforeDigest = liveDigest
	switch status {
	case sync.StatusInSync:
		if srcDigest != "" && srcDigest == liveDigest && srcDigest != d.Skill.BaselineDigest {
			// Independent convergence: re-confirm under the held Store
			// lock (fresh Skill and Source facts, fresh live sample) and
			// advance the Baseline through the durable journal.
			status, stale, err := a.convergeIfConfirmedLocked(ctx, d.Skill.ID)
			if err != nil {
				item.Result = sync.ResultFailed
				item.ErrorCode, item.ErrorMessage = errorCodeOf(err), err.Error()
				return finish()
			}
			item.Status, item.Stale = status, stale
			if err := a.persistSyncStatus(d.Skill.ID, status, stale); err != nil {
				item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
				return finish()
			}
		}
		item.Result = sync.ResultNoOp
	case sync.StatusSourceChanged:
		entry, _ := sourceEntryFacts(src, d.Binding.RelativeDir)
		if entry == nil {
			item.Result, item.ErrorCode, item.ErrorMessage = sync.ResultFailed, CodeInternal,
				"the changed Source entry vanished from the fresh observation"
			return finish()
		}
		// The locked evaluation's live digest is the replace guard: the
		// replacement below must displace exactly that digest, so a local
		// edit landing in between is refused instead of overwritten.
		// A replace that leaves an unresolved Store operation intent
		// propagates the block so the batch stops every later Store write.
		blocked = a.actSourceChanged(ctx, src, d, *entry, &item, liveDigest).blocked
	case sync.StatusStoreChanged, sync.StatusConflict, sync.StatusStoreMissing, sync.StatusStoreInvalid:
		item.Result = sync.ResultSkipped
	case sync.StatusSourceMissing, sync.StatusSourceInvalid:
		item.Result = sync.ResultBlocked
		item.ErrorMessage = sourceProblemMessage(sync.Status(status), d.Binding.RelativeDir)
	default:
		item.Result, item.ErrorCode, item.ErrorMessage = sync.ResultFailed, CodeInternal, "unknown Sync Status "+string(status)
	}
	return finish()
}

// actSourceChanged materializes the changed Source entry and replaces the
// Store content through the durable journal; it fills the item outcome,
// leaves the latest-outcome recording to the caller, and returns the
// journal failure so an unresolved Store operation intent can block the
// rest of the batch. expectedLive is the live digest the locked evaluation
// sampled: the replacement journals it as the displaced digest, and a
// local edit landing between the evaluation and the replace is refused and
// reclassified as a conflict instead of being auto-overwritten.
func (a *App) actSourceChanged(ctx context.Context, src *source.Source, d state.SkillDetail, entry source.Entry, item *SyncItemResult, expectedLive string) syncFailure {
	dir, digest, err := a.materializeSyncContent(ctx, src, entry, false)
	if err != nil {
		item.Result, item.ErrorMessage = sync.ResultFailed, err.Error()
		item.ErrorCode = CodeSyncFailed
		if ctx.Err() != nil {
			item.ErrorCode = CodeCancelled
		}
		return syncFailure{code: item.ErrorCode, message: item.ErrorMessage}
	}
	defer os.RemoveAll(dir)
	fail := a.replaceSkillContent(ctx, &d, src, &entry, dir, digest, modeReplaceAdvance, "sync", expectedLive)
	if fail.code != "" {
		if fail.liveChanged && !fail.committed {
			// The live tree moved between the locked evaluation and the
			// replace: never auto-overwrite it. The relationship is
			// re-evaluated from the same locked facts and reported as a
			// conflict the operator resolves explicitly.
			status, stale, _, _, eerr := a.evaluateSkillSync(ctx, d, src)
			if eerr != nil {
				item.Result = sync.ResultFailed
				item.ErrorCode, item.ErrorMessage = CodeInternal, "re-evaluating after the Store changed: "+eerr.Error()
				return syncFailure{code: CodeInternal, message: eerr.Error()}
			}
			item.Result = sync.ResultSkipped
			item.Status, item.Stale = status, stale
			if perr := a.persistSyncStatus(d.Skill.ID, status, stale); perr != nil {
				item.Result = sync.ResultFailed
				item.ErrorCode, item.ErrorMessage = CodeInternal, perr.Error()
				return syncFailure{code: CodeInternal, message: perr.Error()}
			}
			return syncFailure{}
		}
		item.Result = sync.ResultFailed
		item.ErrorCode, item.ErrorMessage = fail.code, fail.message
		if fail.committed {
			item.AfterDigest = digest
		}
		return fail
	}
	item.Result = sync.ResultUpdated
	item.AfterDigest = digest
	item.Status, item.Stale = sync.StatusInSync, false
	if err := a.persistSyncStatus(d.Skill.ID, sync.StatusInSync, false); err != nil {
		item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
	}
	return syncFailure{}
}

// persistSyncOutcome records one action's outcome; a recording failure is
// reported loudly instead of silently losing the latest outcome.
func (a *App) persistSyncOutcome(skillID int64, item SyncItemResult, started time.Time) error {
	now := time.Now().UTC()
	out := state.SyncOutcome{
		Action: item.Action, Result: item.Result, StartedAt: started, CompletedAt: now,
		BeforeDigest: item.BeforeDigest, AfterDigest: item.AfterDigest,
		Revision: item.Revision, Error: item.ErrorMessage,
	}
	if err := state.UpdateSyncOutcome(a.db, skillID, out); err != nil {
		return Errorf(CodeInternal, "recording the sync outcome of Skill %d: %v", skillID, err)
	}
	return nil
}

// sourceProblemMessage explains one blocked state for the outcome.
func sourceProblemMessage(status sync.Status, relativeDir string) string {
	switch status {
	case sync.StatusSourceMissing:
		return fmt.Sprintf("the Source entry %q is no longer in the Inventory", relativeDir)
	case sync.StatusSourceInvalid:
		return fmt.Sprintf("the Source entry %q failed Skill validation", relativeDir)
	}
	return "the Source entry cannot be retrieved"
}

// errorCodeOf maps a typed app error to its code.
func errorCodeOf(err error) string {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return CodeInternal
}
