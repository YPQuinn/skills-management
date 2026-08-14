package app

import (
	"context"
	"errors"
	"os"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// baselineRefresh runs the Baseline-only refresh through the durable Store
// journal: intent → staging → SQLite commit (Baseline digest, Binding
// acceptance, and the operation's phase in one transaction) → terminal
// finalize (the witnessed Baseline swap and the receipt-bound evidence
// cleanup). The caller holds the Store exclusive lock with recovery
// resolved; the Skill facts are re-read under the lock. A pre-commit
// failure discards only the operation's own staging; a post-commit failure
// keeps the committed row and the next recovery converges.
func (a *App) baselineRefresh(ctx context.Context, skillID int64, src *source.Source, dir, digest string) syncFailure {
	if err := ctx.Err(); err != nil {
		return syncFailure{code: CodeCancelled, message: err.Error()}
	}
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		return syncFailure{code: CodeInternal, message: "reading the Skill under the Store lock: " + err.Error()}
	}
	if d.Binding == nil {
		return syncFailure{code: CodeConflict, message: "Skill " + d.Skill.Slug + " is no longer bound to a Source"}
	}
	if d.Skill.BaselineDigest == digest {
		return syncFailure{}
	}
	// The pre-commit authority check: the FS Baseline must carry exactly
	// the recorded digest before the journal commits, so a foreign or
	// tampered Baseline fails this refresh cleanly instead of surfacing as
	// a blocking post-commit recovery failure.
	if err := a.store.VerifyBaselineDigest(ctx, d.Skill.ID, d.Skill.BaselineDigest); err != nil {
		if errors.Is(err, skillstore.ErrAmbiguous) {
			return syncFailure{code: CodeRecovery, message: err.Error()}
		}
		return syncFailure{code: CodeInternal, message: err.Error()}
	}
	op := skillstore.Operation{
		SkillID: d.Skill.ID, Slug: d.Skill.Slug, Kind: skillstore.KindBaseline,
		OldDigest: d.Skill.BaselineDigest, NewDigest: digest,
	}
	op, err = state.InsertOperation(a.db, op)
	if err != nil {
		return syncFailure{code: CodeInternal, message: "persisting the operation intent: " + err.Error()}
	}
	staged, err := a.store.Stage(ctx, op.ID, dir, digest, true)
	if err != nil {
		if errors.Is(err, skillstore.ErrAmbiguous) {
			return syncFailure{code: CodeRecovery, message: "staging is ambiguous and every candidate is preserved: " + err.Error(), blocked: true}
		}
		if fail := a.abortBaselineStaging(ctx, op, nil); fail.code != "" {
			return fail
		}
		code := CodeSyncFailed
		if ctx.Err() != nil {
			code = CodeCancelled
		}
		return syncFailure{code: code, message: err.Error()}
	}
	if a.commitHook != nil {
		a.commitHook(false)
	}
	if err := ctx.Err(); err != nil {
		if fail := a.abortBaselineStaging(context.WithoutCancel(ctx), op, &staged); fail.code != "" {
			return fail
		}
		return syncFailure{code: CodeCancelled, message: err.Error()}
	}
	if err := state.CommitBaselineAdvance(a.db, op, d.Skill.ID, d.Skill.BaselineDigest, digest, src.LastCommit); err != nil {
		if fail := a.abortBaselineStaging(context.WithoutCancel(ctx), op, &staged); fail.code != "" {
			return fail
		}
		return syncFailure{code: CodeInternal, message: "committing the Baseline advance: " + err.Error()}
	}
	if a.commitHook != nil {
		a.commitHook(true)
	}
	op.SkillID = d.Skill.ID
	if err := a.finalizeTerminal(context.WithoutCancel(ctx), op); err != nil {
		return syncFailure{code: CodeRecovery, message: "committed operation " + itoa(op.ID) + " could not be finalized and cleared: " + err.Error(), blocked: true, committed: true}
	}
	return syncFailure{}
}

// abortBaselineStaging discards the staged content of a pre-commit
// Baseline-refresh failure (when a Stage proof exists) and clears the
// intent; the Baseline and the live tree were never mutated.
func (a *App) abortBaselineStaging(ctx context.Context, op skillstore.Operation, staged *skillstore.StagedProof) syncFailure {
	if staged != nil {
		if err := a.store.DiscardStaging(op.ID, *staged); err != nil {
			return syncFailure{code: CodeRecovery, message: "operation " + itoa(op.ID) + " staging could not be provably discarded: " + err.Error(), blocked: true}
		}
	}
	if aerr := a.abortOpCleanup(ctx, op); aerr != nil {
		return syncFailure{code: CodeRecovery, message: "operation " + itoa(op.ID) + " could not be aborted: " + aerr.Error(), blocked: true}
	}
	return syncFailure{}
}

// convergeIfConfirmedLocked re-reads the Skill and the persisted Source
// facts under the held Store lock and re-samples the live tree: only when
// the locked facts confirm independent convergence (Source equals Store,
// Baseline lags) does it advance the Baseline through the durable journal.
// The returned status and staleness come from the locked facts, so in_sync
// is never marked from a stale outside sample.
func (a *App) convergeIfConfirmedLocked(ctx context.Context, skillID int64) (sync.Status, bool, error) {
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		return "", false, Errorf(CodeInternal, "reading Skill %d under the Store lock: %v", skillID, err)
	}
	if d.Binding == nil {
		return sync.StatusUnbound, false, nil
	}
	src, err := state.GetSource(a.db, d.Binding.SourceID)
	if err != nil {
		return "", false, Errorf(CodeInternal, "reading Source %d under the Store lock: %v", d.Binding.SourceID, err)
	}
	status, stale, srcDigest, liveDigest, err := a.evaluateSkillSync(ctx, *d, src)
	if err != nil {
		return "", false, Errorf(CodeInternal, "evaluating Skill %d: %v", skillID, err)
	}
	if status != sync.StatusInSync || srcDigest == "" || srcDigest != liveDigest || srcDigest == d.Skill.BaselineDigest {
		return status, stale, nil
	}
	entry, _ := sourceEntryFacts(src, d.Binding.RelativeDir)
	if entry == nil {
		return status, stale, Errorf(CodeInternal, "the converged Source entry vanished from the observation")
	}
	tmp, err := os.MkdirTemp("", "skillctl-baseline-*")
	if err != nil {
		return status, stale, Errorf(CodeInternal, "creating Baseline content: %v", err)
	}
	defer os.RemoveAll(tmp)
	digest, err := a.materialize(ctx, src.Locator, src.LastCommit, *entry, a.workDir(), tmp, true)
	if err != nil {
		if errors.Is(err, source.ErrContentChanged) {
			return status, stale, Errorf(CodeConflict, "the Source content changed while the check was running; check the Skill again")
		}
		if ctx.Err() != nil {
			return status, stale, Errorf(CodeCancelled, "%s", err.Error())
		}
		return status, stale, Errorf(CodeSyncFailed, "materializing the converged Source content: %v", err)
	}
	if digest != srcDigest {
		return status, stale, Errorf(CodeConflict, "the Source content changed while the check was running; check the Skill again")
	}
	if fail := a.baselineRefresh(ctx, skillID, src, tmp, digest); fail.code != "" {
		return status, stale, Errorf(fail.code, "%s", fail.message)
	}
	return status, stale, nil
}
