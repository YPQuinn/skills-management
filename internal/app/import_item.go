package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// importItem processes one selected entry independently: same Source Binding
// is an already_imported no-op; a slug claimed by a different managed Skill
// is skipped_conflict unless explicit Replace (which retains the existing
// Skill's id and slug); an unmanaged live Store directory is never
// overwritten. Every mutation follows the durable sequence materialize →
// intent → stage → install → SQLite commit → non-cancelled finalize →
// intent cleanup, with per-phase cleanup that preserves any state it cannot
// prove safe to remove. Cancellation is honored before the SQLite commit;
// once the commit succeeds, finalization runs with a non-cancelled context
// and any incomplete finalization leaves a recoverable committed intent.
// The outcome reports whether an unresolved Store operation intent remains,
// which blocks every later Store mutation in the batch.
func (a *App) importItem(ctx context.Context, src *source.Source, entry source.Entry, sel ImportSelector, allowLarge bool) importOutcome {
	if err := ctx.Err(); err != nil {
		return importOutcome{result: failedImport(entry, CodeCancelled, "%v", err)}
	}
	bound, err := state.GetSkillBySourceEntry(a.db, src.ID, entry.RelativeDir)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return importOutcome{result: failedImport(entry, CodeInternal, "reading the Source Binding: %v", err)}
	}
	if bound != nil {
		item := ImportItemResult{
			Status: StatusAlreadyImported, RelativeDir: entry.RelativeDir,
			Slug: bound.Skill.Slug, SkillID: bound.Skill.ID,
		}
		if sel.Slug != nil {
			item.RequestedSlug = *sel.Slug
		}
		return importOutcome{result: item}
	}

	requested := ""
	if sel.Slug != nil {
		requested = *sel.Slug
	} else {
		slug, err := defaultSlug(entry.Name)
		if err != nil {
			return importOutcome{result: failedImport(entry, CodeInvalidArgument, "%v", err)}
		}
		requested = slug
	}
	existing, err := state.GetSkillDetailBySlug(a.db, requested)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return importOutcome{result: failedImportMeta(entry, requested, 0, nil, CodeInternal, "reading Skill %q: %v", requested, err)}
	}
	if existing != nil && !sel.Replace {
		impact, err := a.replaceImpact(existing.Skill.ID)
		if err != nil {
			return importOutcome{result: failedImportMeta(entry, requested, existing.Skill.ID, nil, CodeInternal, "reading the replace impact: %v", err)}
		}
		return importOutcome{result: ImportItemResult{
			Status: StatusSkippedConflict, RelativeDir: entry.RelativeDir,
			RequestedSlug: requested, Slug: existing.Skill.Slug,
			SkillID: existing.Skill.ID, Replaces: previewSkill(existing), Impact: impact,
		}}
	}
	// Once an existing managed Skill is the Replace target, its id, preview,
	// and impact are known and belong on every later failure outcome.
	skillID := int64(0)
	var replaces *Skill
	var impact *ReplaceImpact
	if existing != nil {
		skillID = existing.Skill.ID
		replaces = previewSkill(existing)
		impact, err = a.replaceImpact(skillID)
		if err != nil {
			return importOutcome{result: failedImportMeta(entry, requested, skillID, replaces, CodeInternal, "reading the replace impact: %v", err)}
		}
	}

	// Materialize the exact fresh observation into caller-owned temp
	// content outside the Skill Store; the materializer re-proves the entry
	// digest.
	tmp, err := os.MkdirTemp("", "skillctl-import-*")
	if err != nil {
		return importOutcome{result: failedImportMeta(entry, requested, skillID, replaces, CodeInternal, "creating temp content: %v", err)}
	}
	digest, err := a.materialize(ctx, src.Locator, src.LastCommit, entry, a.workDir(), tmp, allowLarge)
	if err != nil {
		os.RemoveAll(tmp)
		code := CodeImportFailed
		if ctx.Err() != nil {
			code = CodeCancelled
		}
		return importOutcome{result: failedImportMeta(entry, requested, skillID, replaces, code, "%v", err)}
	}
	defer os.RemoveAll(tmp)

	op := skillstore.Operation{Slug: requested, Kind: skillstore.KindImport, NewDigest: digest}
	if existing != nil {
		op = skillstore.Operation{
			SkillID: existing.Skill.ID, Slug: existing.Skill.Slug,
			Kind: skillstore.KindReplace, OldDigest: existing.Skill.StoreDigest,
			NewDigest: digest,
		}
	}
	// The durable intent is persisted before any Store filesystem mutation.
	op, err = state.InsertOperation(a.db, op)
	if err != nil {
		return importOutcome{result: failedImportMeta(entry, requested, skillID, replaces, CodeInternal, "persisting the operation intent: %v", err)}
	}

	staged, err := a.store.Stage(ctx, op.ID, tmp, digest, allowLarge)
	if err != nil {
		// Stage never mutates the live path. Ordinary Stage failures have
		// already provably removed the operation's own staging with the
		// retained identities, so the intent can be durably aborted and
		// cleared without any further discard. ErrAmbiguous (a pre-existing
		// or swapped operation directory, or a cleanup that cannot be
		// proven) preserves everything: the intent stays pending and every
		// candidate survives for the next recovery.
		code := CodeImportFailed
		if ctx.Err() != nil {
			code = CodeCancelled
		}
		if errors.Is(err, skillstore.ErrAmbiguous) {
			return importOutcome{result: failedImportMeta(entry, requested, skillID, replaces, CodeRecovery,
				"staging %q is ambiguous and every candidate is preserved: %v", requested, err), blocked: true}
		}
		return a.abortAfterCleanup(ctx, entry, op, requested, skillID, replaces, code, "staging %q: %v", requested, err)
	}
	if err := a.store.Install(ctx, op, &staged); err != nil {
		if errors.Is(err, skillstore.ErrAmbiguous) {
			// Install could not prove its own transition: every candidate is
			// preserved, the intent stays pending, and later recovery decides.
			return importOutcome{result: failedImportMeta(entry, requested, skillID, replaces, CodeRecovery,
				"installing %q is ambiguous and every candidate is preserved: %v", requested, err), blocked: true}
		}
		// A non-ambiguous Install failure happened before any live mutation
		// (digest or identity refusal, changed live tree, ErrUnmanaged,
		// ErrMissing, or a copy error), so the staging is discarded with the
		// opaque proof — never with a fresh sample of the operation name.
		// An unprovable discard keeps the intent pending with everything
		// preserved; a proven discard durably marks the intent aborted.
		if derr := a.store.DiscardStaging(op.ID, staged); derr != nil {
			return importOutcome{result: failedImportMeta(entry, requested, skillID, replaces, CodeRecovery,
				"operation %d staging could not be provably discarded; every candidate is preserved: %v", op.ID, derr), blocked: true}
		}
		code := CodeImportFailed
		if errors.Is(err, skillstore.ErrUnmanaged) {
			code = CodeConflict
		}
		return a.abortAfterCleanup(ctx, entry, op, requested, skillID, replaces, code, "%v", err)
	}
	if err := ctx.Err(); err != nil {
		return a.failAfterMutation(ctx, entry, op, requested, skillID, replaces, err)
	}
	if a.commitHook != nil {
		a.commitHook(false)
	}
	if err := ctx.Err(); err != nil {
		return a.failAfterMutation(ctx, entry, op, requested, skillID, replaces, err)
	}

	now := time.Now().UTC()
	s := state.Skill{
		Slug: requested, Name: entry.Name, Description: entry.Description,
		StoreDigest: digest, BaselineDigest: digest, UpdatedAt: now,
	}
	b := state.Binding{
		SourceID: src.ID, RelativeDir: entry.RelativeDir, Digest: digest,
		SourceCommit: src.LastCommit, ImportedAt: now,
	}
	if existing != nil {
		s.ID = existing.Skill.ID
		s.CreatedAt = existing.Skill.CreatedAt
		err = state.CommitReplace(a.db, s, b, op.ID)
	} else {
		s.CreatedAt = now
		skillID, err = state.CommitImport(a.db, s, b, op.ID)
	}
	if err != nil {
		return a.failAfterMutation(ctx, entry, op, requested, skillID, replaces, err)
	}
	if existing != nil {
		skillID = existing.Skill.ID
	}
	op.SkillID = skillID

	// The commit has completed: finalization is deterministic and uses a
	// non-cancelled context; a failure leaves the committed (or receipt-
	// finalized) intent for the next write's recovery.
	if a.commitHook != nil {
		a.commitHook(true)
	}
	if err := a.finalizeTerminal(context.WithoutCancel(ctx), op); err != nil {
		return importOutcome{result: failedImportMeta(entry, requested, skillID, replaces, CodeRecovery,
			"committed operation %d could not be finalized and cleared: %v", op.ID, err), blocked: true}
	}
	item := ImportItemResult{
		Status: StatusImported, RelativeDir: entry.RelativeDir,
		RequestedSlug: requested, Slug: requested, SkillID: skillID,
	}
	if existing != nil {
		item.Status = StatusReplaced
		item.Replaces = replaces
		item.Impact = impact
	}
	return importOutcome{result: item}
}

// abortAfterCleanup durably marks a pre-install failure whose staging was
// provably removed (by Stage itself or by DiscardStaging) as aborted and
// clears the intent; a deletion failure keeps the aborted row for the next
// recovery and blocks later Store writes. The approved durable ordering
// runs first: the full-identity pending→aborted CAS, then Store Abort
// re-verifies the operation/recovery evidence absent — it never touches
// live content — then the full-identity aborted-row CAS clears the intent.
// If Abort fails, the aborted row remains with every candidate preserved.
func (a *App) abortAfterCleanup(ctx context.Context, entry source.Entry, op skillstore.Operation, slug string, skillID int64, replaces *Skill, code, format string, args ...any) importOutcome {
	if err := state.MarkOperationAborted(a.db, op); err != nil {
		return importOutcome{result: failedImportMeta(entry, slug, skillID, replaces, CodeRecovery,
			"operation %d could not be marked aborted: %v", op.ID, err), blocked: true}
	}
	op.Phase = skillstore.PhaseAborted
	if err := a.store.Abort(context.WithoutCancel(ctx), op); err != nil {
		return importOutcome{result: failedImportMeta(entry, slug, skillID, replaces, CodeRecovery,
			"operation %d evidence absence could not be proven for the abort: %v", op.ID, err), blocked: true}
	}
	return a.outcomeAfterCleanup(entry, op, slug, skillID, replaces, code, format, args...)
}

// outcomeAfterCleanup finishes a pre-mutation failure whose staging was
// provably discarded and whose evidence absence Store Abort certified: the
// intent is cleared by the full-identity/phase CAS; a deletion failure
// keeps the intent (the next recovery finishes it) and blocks later Store
// writes.
func (a *App) outcomeAfterCleanup(entry source.Entry, op skillstore.Operation, slug string, skillID int64, replaces *Skill, code, format string, args ...any) importOutcome {
	if err := a.deleteOpIntent(op); err != nil {
		return importOutcome{result: failedImportMeta(entry, slug, skillID, replaces, CodeRecovery,
			"operation %d cleaned up but its intent could not be cleared: %v", op.ID, err), blocked: true}
	}
	return importOutcome{result: failedImportMeta(entry, slug, skillID, replaces, code, format, args...)}
}

// failAfterMutation unwinds a pending operation whose live path may have
// been mutated: the terminal restore protocol runs with a non-cancelled
// context (prepare, durable receipt, receipt-bound evidence cleanup, exact
// terminal-row deletion), and anything that cannot be proven is preserved
// as a pending intent for the next recovery. A cleanup failure keeps the
// intent (pending, or restored with its receipt) and blocks later Store
// writes. Cancellation maps to CodeCancelled, other causes to
// CodeImportFailed.
func (a *App) failAfterMutation(ctx context.Context, entry source.Entry, op skillstore.Operation, slug string, skillID int64, replaces *Skill, cause error) importOutcome {
	if err := a.restoreTerminal(context.WithoutCancel(ctx), op); err != nil {
		return importOutcome{result: failedImportMeta(entry, slug, skillID, replaces, CodeRecovery,
			"operation %d could not be restored and cleared: %v", op.ID, err), blocked: true}
	}
	code := CodeImportFailed
	if ctx.Err() != nil {
		code = CodeCancelled
	}
	return importOutcome{result: failedImportMeta(entry, slug, skillID, replaces, code, "%v", cause)}
}

// previewSkill converts one state detail into the replacement-preview view
// of the existing managed Skill and its Source Binding.
func previewSkill(d *state.SkillDetail) *Skill {
	s := appSkill(*d)
	return &s
}
