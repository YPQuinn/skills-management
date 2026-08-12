package app

import (
	"context"
	"fmt"

	"skillctl/internal/source"
)

// Stable per-item failure codes reported in ImportItemResult.ErrorCode.
const (
	CodeImportFailed = "import_failed"
	CodeCancelled    = "cancelled"
)

// importSelection pairs one resolved Inventory entry with its selector
// options; for an All request the options are zero-valued.
type importSelection struct {
	Entry    source.Entry
	Selector ImportSelector
}

// resolveImportSelections resolves the validated request against the fresh
// observation and pairs every entry with its selector options. Explicit
// selectors keep their request order, so the i-th resolved entry
// corresponds to the i-th selector; All expands to every entry sorted by
// relative directory with zero-value options (Replace and slug overrides
// are impossible with All by validation).
func resolveImportSelections(in ImportSkillsInput, entries []source.Entry) ([]importSelection, error) {
	resolved, err := resolveImportSelectors(in, entries)
	if err != nil {
		return nil, err
	}
	out := make([]importSelection, len(resolved))
	for i, e := range resolved {
		sel := ImportSelector{}
		if !in.All {
			if len(in.Selectors) != len(resolved) {
				return nil, fmt.Errorf("selector resolution lost request order")
			}
			sel = in.Selectors[i]
		}
		out[i] = importSelection{Entry: e, Selector: sel}
	}
	return out, nil
}

// ImportSkills imports one batch of Inventory entries from one Source. The
// batch performs exactly one fresh coherent observation under the per-Source
// lock and persists it before any Store work; unavailability blocks the
// whole batch as CodeSourceUnavailable without touching the Store. Selector
// and request failures before item processing are top-level typed errors.
// After the Store exclusive lock is taken and every open operation is
// recovered, each selected entry is processed independently with its own
// outcome; one item's failure never rolls back its siblings, and a valid
// begun batch always returns every item outcome plus a summary. Top-level
// errors are only request, Source, lock, and recovery failures before item
// processing.
func (a *App) ImportSkills(ctx context.Context, in ImportSkillsInput) (*ImportSkillsResult, error) {
	if err := validateImportInput(in); err != nil {
		return nil, Errorf(CodeInvalidArgument, "%v", err)
	}
	// Exactly one fresh coherent observation per batch, under the per-Source
	// lock and before any Store work; the lock is released here.
	src, err := a.observeSource(ctx, in.SourceID)
	if err != nil {
		return nil, err
	}
	if !src.Available {
		return nil, Errorf(CodeSourceUnavailable, "Source %s is not reachable: %s", src.Location, src.LastError)
	}
	selections, err := resolveImportSelections(in, src.Entries)
	if err != nil {
		return nil, Errorf(CodeInvalidArgument, "%v", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The Source lock was released; the Store exclusive lock is taken for
	// recovery and every Store write, so the two locks are never held
	// together and cannot deadlock.
	held, err := a.acquireStoreLock()
	if err != nil {
		return nil, err
	}
	defer held.Unlock()
	if err := a.recoverOpenOperations(context.WithoutCancel(ctx)); err != nil {
		return nil, err
	}
	// Once an item leaves an unresolved Store operation intent (a failed
	// Restore, Finalize, or intent deletion), recovery-blocks-writes applies
	// inside the batch: no later Store mutation runs, and every remaining
	// selection is reported failed/CodeRecovery with a clear message.
	items := make([]ImportItemResult, 0, len(selections))
	blocked := false
	for _, sel := range selections {
		if blocked {
			items = append(items, blockedOutcome(sel).result)
			continue
		}
		out := a.importItem(ctx, src, sel.Entry, sel.Selector, in.AllowLarge)
		items = append(items, out.result)
		blocked = out.blocked
	}
	return &ImportSkillsResult{Items: items, Summary: SummarizeImportItems(items)}, nil
}
