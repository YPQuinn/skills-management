package app

import (
	"context"
	"errors"
	"os"
	"time"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// syncOutcome is one item's result plus whether an unresolved Store
// operation intent remains after it; a true blocked stops every later
// Store mutation in the batch.
type syncOutcome struct {
	result  SyncItemResult
	blocked bool
}

// syncReplaceMode selects the journal shape of one Store-content
// replacement.
type syncReplaceMode int

const (
	// modeReplaceAdvance is the ordinary replace: the Baseline advances to
	// the new content at finalization.
	modeReplaceAdvance syncReplaceMode = iota
	// modeReplaceKeep is the rollback replace: the Baseline is retained and
	// the pre-replacement live tree becomes the new previous snapshot.
	modeReplaceKeep
	// modeRepairImport installs into an absent live slot: the existing
	// Skill row and Binding are updated and the journal advances the
	// recorded Baseline to the accepted content in the same finalization.
	modeRepairImport
)

// syncFailure is the structured failure of one replacement attempt.
// committed reports that the SQLite commit completed before the failure
// (a finalize or terminal-cleanup failure), so the accepted content and
// digests are durably in effect even though the outcome failed.
// liveChanged reports the replace guard fired: the live tree no longer
// carries the digest the replacement was prepared against, so the caller
// reclassifies the Skill instead of overwriting a local edit.
type syncFailure struct {
	code, message string
	blocked       bool
	committed     bool
	liveChanged   bool
}

// replaceSkillContent runs the durable journaled Store replacement of one
// committed Skill with the content at contentDir (digest-verified against
// digest): intent → stage → install → SQLite commit → non-cancelled
// finalize, exactly the import sequence. mode selects the journal shape and
// the SQLite commit; src and entry carry the acceptance identity for the
// advancing modes and are nil for a rollback. expectedLive is the digest of
// the live tree the caller sampled under the held Store lock (the locked
// evaluation for automatic sync, the confirmation sample for explicit
// Accept Source): the advancing replace journals it as the displaced
// digest, so Install's digest proof refuses any local edit that landed
// afterwards — a changed live tree surfaces as a liveChanged failure
// instead of being overwritten. An empty expectedLive only occurs when the
// locked sample could not digest the live slot (a plain file): the journal
// then displaces it by physical identity without a snapshot. The returned
// failure is empty on success.
func (a *App) replaceSkillContent(ctx context.Context, d *state.SkillDetail, src *source.Source, entry *source.Entry, contentDir, digest string, mode syncReplaceMode, reason string, expectedLive string) syncFailure {
	if err := ctx.Err(); err != nil {
		return syncFailure{code: CodeCancelled, message: err.Error()}
	}
	op := skillstore.Operation{
		SkillID: d.Skill.ID, Slug: d.Skill.Slug, Kind: skillstore.KindReplace,
		NewDigest: digest, BaselineMode: skillstore.BaselineAdvance,
	}
	allowLarge := false
	switch mode {
	case modeReplaceKeep:
		// Rollback journals the persisted digest: the caller re-checked
		// that the live tree still carries it (the confirmation re-check
		// of decision 05), and Install re-proves it before anything moves.
		op.OldDigest = d.Skill.StoreDigest
		op.BaselineMode = skillstore.BaselineKeep
		allowLarge = true // the snapshot was previously accepted content
	case modeRepairImport:
		// A repair imports into the absent live slot and advances the
		// Baseline in the same journal: the operation records the expected
		// pre-operation Baseline digest, so the finalization proves and
		// displaces exactly it through the shared Baseline machinery.
		op = skillstore.Operation{Slug: d.Skill.Slug, Kind: skillstore.KindImport, NewDigest: digest,
			BaselineMode: skillstore.BaselineAdvance, BaselineDigest: d.Skill.BaselineDigest}
	default:
		// expectedLive is the digest the caller proved under the held
		// Store lock; the journal records it as the displaced live digest
		// so Install refuses any local edit that landed since (a stale
		// replacement must never overwrite Store edits). Only a live slot
		// that cannot be digested at all (a plain file) is displaced by
		// physical identity without a snapshot.
		if expectedLive == "" {
			dir, err := a.store.SkillDir(d.Skill.Slug)
			if err != nil {
				return syncFailure{code: CodeInternal, message: err.Error()}
			}
			info, err := os.Lstat(dir)
			switch {
			case errors.Is(err, os.ErrNotExist):
				return syncFailure{code: CodeConflict, message: "the Store content is missing"}
			case err != nil:
				return syncFailure{code: CodeConflict, message: "the Store content cannot be read"}
			case info.IsDir():
				// The locked sample could not digest a directory: it is not
				// a replaceable Skill tree.
				return syncFailure{code: CodeConflict, message: "the Store content cannot be read"}
			default:
				op.OldDigest = ""
				op.BaselineDigest = d.Skill.BaselineDigest
			}
		} else {
			op.OldDigest = expectedLive
			if d.Skill.BaselineDigest != expectedLive {
				op.BaselineDigest = d.Skill.BaselineDigest
			}
		}
	}
	op, err := state.InsertOperation(a.db, op)
	if err != nil {
		return syncFailure{code: CodeInternal, message: "persisting the operation intent: " + err.Error()}
	}
	if a.afterIntentHook != nil {
		a.afterIntentHook()
	}
	staged, err := a.store.Stage(ctx, op.ID, contentDir, digest, allowLarge)
	if err != nil {
		if errors.Is(err, skillstore.ErrAmbiguous) {
			return syncFailure{code: CodeRecovery, message: "staging is ambiguous and every candidate is preserved: " + err.Error(), blocked: true}
		}
		if aerr := a.abortOpCleanup(ctx, op); aerr != nil {
			return syncFailure{code: CodeRecovery, message: "operation " + itoa(op.ID) + " could not be aborted: " + aerr.Error(), blocked: true}
		}
		code := CodeSyncFailed
		if ctx.Err() != nil {
			code = CodeCancelled
		}
		return syncFailure{code: code, message: err.Error()}
	}
	if err := a.store.Install(ctx, op, &staged); err != nil {
		if errors.Is(err, skillstore.ErrAmbiguous) {
			return syncFailure{code: CodeRecovery, message: "the install is ambiguous and every candidate is preserved: " + err.Error(), blocked: true}
		}
		if derr := a.store.DiscardStaging(op.ID, staged); derr != nil {
			return syncFailure{code: CodeRecovery, message: "operation " + itoa(op.ID) + " staging could not be provably discarded: " + derr.Error(), blocked: true}
		}
		if aerr := a.abortOpCleanup(ctx, op); aerr != nil {
			return syncFailure{code: CodeRecovery, message: "operation " + itoa(op.ID) + " could not be aborted: " + aerr.Error(), blocked: true}
		}
		code := CodeSyncFailed
		if errors.Is(err, skillstore.ErrUnmanaged) || errors.Is(err, skillstore.ErrMissing) || errors.Is(err, skillstore.ErrLiveChanged) {
			code = CodeConflict
		}
		if ctx.Err() != nil {
			code = CodeCancelled
		}
		return syncFailure{code: code, message: err.Error(), liveChanged: errors.Is(err, skillstore.ErrLiveChanged)}
	}
	if err := ctx.Err(); err != nil {
		return a.syncFailAfterMutation(ctx, op, err)
	}
	if a.commitHook != nil {
		a.commitHook(false)
	}
	if err := ctx.Err(); err != nil {
		return a.syncFailAfterMutation(ctx, op, err)
	}
	now := time.Now().UTC()
	if err := a.commitSyncContent(op, *d, src, entry, digest, mode, reason, now); err != nil {
		return a.syncFailAfterMutation(ctx, op, err)
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

// commitSyncContent persists the SQLite half of the installed transition.
func (a *App) commitSyncContent(op skillstore.Operation, d state.SkillDetail, src *source.Source, entry *source.Entry, digest string, mode syncReplaceMode, reason string, now time.Time) error {
	switch mode {
	case modeReplaceAdvance:
		s := state.Skill{
			ID: d.Skill.ID, Slug: d.Skill.Slug, Name: entry.Name, Description: entry.Description,
			StoreDigest: digest, BaselineDigest: digest, CreatedAt: d.Skill.CreatedAt, UpdatedAt: now,
		}
		b := state.Binding{
			SkillID: d.Skill.ID, SourceID: src.ID, RelativeDir: entry.RelativeDir,
			Digest: digest, SourceCommit: src.LastCommit, ImportedAt: now,
		}
		return state.CommitSyncReplace(a.db, s, b, op.ID, reason)
	case modeReplaceKeep:
		return state.CommitRollback(a.db, state.Skill{
			ID: d.Skill.ID, Slug: d.Skill.Slug,
			StoreDigest: digest, CreatedAt: d.Skill.CreatedAt, UpdatedAt: now,
		}, op.ID)
	case modeRepairImport:
		return state.CommitRepairImport(a.db, state.Skill{
			ID: d.Skill.ID, Slug: d.Skill.Slug, Name: entry.Name, Description: entry.Description,
			StoreDigest: digest, BaselineDigest: digest,
			CreatedAt: d.Skill.CreatedAt, UpdatedAt: now,
		}, state.Binding{
			SkillID: d.Skill.ID, SourceID: src.ID, RelativeDir: entry.RelativeDir,
			Digest: digest, SourceCommit: src.LastCommit, ImportedAt: now,
		}, op.ID, d.Skill.BaselineDigest)
	}
	return Errorf(CodeInternal, "unknown sync replace mode")
}

// syncFailAfterMutation unwinds a pending operation whose live path may
// have been mutated: the terminal restore protocol runs with a
// non-cancelled context, and anything that cannot be proven is preserved
// as a pending intent for the next recovery.
func (a *App) syncFailAfterMutation(ctx context.Context, op skillstore.Operation, cause error) syncFailure {
	if err := a.restoreTerminal(context.WithoutCancel(ctx), op); err != nil {
		return syncFailure{code: CodeRecovery, message: "operation " + itoa(op.ID) + " could not be restored and cleared: " + err.Error(), blocked: true}
	}
	code := CodeSyncFailed
	if ctx.Err() != nil {
		code = CodeCancelled
	}
	return syncFailure{code: code, message: cause.Error()}
}

// abortOpCleanup durably marks a pre-install failure whose staging was
// provably removed as aborted and clears the intent; a failure keeps the
// aborted row for the next recovery.
func (a *App) abortOpCleanup(ctx context.Context, op skillstore.Operation) error {
	if err := state.MarkOperationAborted(a.db, op); err != nil {
		return err
	}
	op.Phase = skillstore.PhaseAborted
	if err := a.store.Abort(context.WithoutCancel(ctx), op); err != nil {
		return err
	}
	return a.deleteOpIntent(op)
}

// materializeSyncContent materializes one fresh-observation entry into a
// caller-owned temp dir; the caller removes it.
func (a *App) materializeSyncContent(ctx context.Context, src *source.Source, entry source.Entry, allowLarge bool) (string, string, error) {
	tmp, err := os.MkdirTemp("", "skillctl-sync-*")
	if err != nil {
		return "", "", err
	}
	digest, err := a.materialize(ctx, src.Locator, src.LastCommit, entry, a.workDir(), tmp, allowLarge)
	if err != nil {
		os.RemoveAll(tmp)
		return "", "", err
	}
	return tmp, digest, nil
}
