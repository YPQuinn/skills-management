package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"skillctl/internal/lock"
	"skillctl/internal/skillstore"
	"skillctl/internal/state"
)

// CodeRecovery is the stable code for Store journal recovery failures: an
// open operation whose state cannot be proven blocks all new Store writes
// while every candidate is preserved.
const CodeRecovery = "recovery_failed"

// storeLockPath is the Store-exclusive cross-process lock, beside state.db
// with the per-Source locks.
func (a *App) storeLockPath() (string, error) {
	dir := filepath.Join(filepath.Dir(a.StateDBPath), "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "store.lock"), nil
}

// acquireStoreLock takes the Store-exclusive cross-process lock, reporting
// contention as CodeLocked. Callers must hold it for the whole Store write
// and never combine it with a held Source lock: imports observe under the
// Source lock first, release it, and only then take the Store lock, so the
// lock order is unambiguous and deadlock-free.
func (a *App) acquireStoreLock() (*lock.Lock, error) {
	path, err := a.storeLockPath()
	if err != nil {
		return nil, Errorf(CodeInternal, "preparing Store lock: %v", err)
	}
	held, err := lock.TryExclusive(path)
	if errors.Is(err, lock.ErrLocked) {
		return nil, Errorf(CodeLocked, "another skillctl process is writing the Skill Store")
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "locking the Skill Store: %v", err)
	}
	return held, nil
}

// recoverOnOpen resolves every unfinished Store operation under the Store
// exclusive lock before the App is exposed, through the same recovery
// sequence every later Store write shares. A proven-unrecoverable
// operation or lock contention refuses the open with the stable
// recovery_failed / locked codes; the caller closes the half-opened App.
func (a *App) recoverOnOpen() error {
	held, err := a.acquireStoreLock()
	if err != nil {
		return err
	}
	defer held.Unlock()
	return a.recoverOpenOperations(context.Background())
}

// recoverOpenOperations resolves every unfinished Store operation intent in
// journal order while the caller holds the Store exclusive lock: committed
// intents run the terminal finalize protocol, pending intents the terminal
// restore protocol, finalized/restored intents resume the receipt-bound
// evidence cleanup, and aborted intents are certified by evidence absence.
// Recovery always runs with a non-cancelled context so it completes
// deterministically. An operation whose state cannot be proven valid stops
// recovery and returns CodeRecovery with all candidates preserved; no new
// Store write proceeds.
func (a *App) recoverOpenOperations(ctx context.Context) error {
	ops, err := state.ListOpenOperations(a.db)
	if err != nil {
		return Errorf(CodeInternal, "listing Store operations: %v", err)
	}
	rc := context.WithoutCancel(ctx)
	for _, op := range ops {
		var err error
		switch op.Phase {
		case skillstore.PhaseCommitted:
			err = a.finalizeTerminal(rc, op)
		case skillstore.PhasePending:
			err = a.restoreTerminal(rc, op)
		case skillstore.PhaseFinalized, skillstore.PhaseRestored:
			err = a.cleanupTerminalRow(rc, op)
		case skillstore.PhaseAborted:
			// The aborted row is certified only by Store Abort proving the
			// operation evidence absent; only then is the row cleared by
			// the full-identity/phase CAS.
			err = a.store.Abort(rc, op)
			if err == nil {
				err = a.deleteOpIntent(op)
			}
		default:
			err = Errorf(CodeRecovery, "operation %d has unknown phase %q", op.ID, op.Phase)
		}
		if err != nil {
			return Errorf(CodeRecovery, "recovering operation %d: %v", op.ID, err)
		}
	}
	return nil
}

// finalizeTerminal runs the terminal finalize protocol for one committed
// operation: the Store performs the semantic finalization and returns the
// opaque receipt, the receipt is durably CAS-bound to the operation row,
// the Store removes the receipt-bound evidence, and the terminal row is
// deleted by the exact receipt CAS. A failure at any step keeps the row
// (committed, or finalized with its receipt) so the next recovery
// converges from the exact persisted state.
func (a *App) finalizeTerminal(ctx context.Context, op skillstore.Operation) error {
	receipt, err := a.store.PrepareFinalize(ctx, op)
	if err != nil {
		return err
	}
	op.Phase = skillstore.PhaseFinalized
	op.Receipt = receipt.Bytes()
	if err := a.persistTerminalReceipt(op); err != nil {
		return err
	}
	if err := a.store.CleanupTerminal(ctx, op, receipt); err != nil {
		return err
	}
	return a.deleteOpTerminal(op)
}

// restoreTerminal runs the terminal restore protocol for one pending
// operation: the Store performs the semantic restore and returns the
// opaque receipt (or none when no operation evidence existed), the receipt
// is durably CAS-bound to the operation row, the Store removes the
// receipt-bound evidence, and the terminal row is deleted by the exact
// receipt CAS. A failure at any step keeps the row (pending, or restored
// with its receipt) so the next recovery converges from the exact
// persisted state.
func (a *App) restoreTerminal(ctx context.Context, op skillstore.Operation) error {
	receipt, err := a.store.PrepareRestore(ctx, op)
	if err != nil {
		return err
	}
	if receipt.Empty() {
		// No operation evidence existed at all: the operation never
		// started (or a previous restore already cleaned everything), so
		// the abort protocol clears the intent without touching live
		// content. The approved durable ordering runs first: the
		// full-identity pending→aborted CAS, then Store Abort verifies
		// the operation/recovery evidence absent, then the full-identity
		// aborted-row CAS deletes the intent. If Abort fails, the aborted
		// row remains with every candidate preserved for the next
		// recovery.
		if err := state.MarkOperationAborted(a.db, op); err != nil {
			return err
		}
		op.Phase = skillstore.PhaseAborted
		if err := a.store.Abort(ctx, op); err != nil {
			return err
		}
		return a.deleteOpIntent(op)
	}
	op.Phase = skillstore.PhaseRestored
	op.Receipt = receipt.Bytes()
	if err := a.persistTerminalReceipt(op); err != nil {
		return err
	}
	if err := a.store.CleanupTerminal(ctx, op, receipt); err != nil {
		return err
	}
	return a.deleteOpTerminal(op)
}

// persistTerminalReceipt durably CAS-binds the terminal receipt of a
// finalized or restored row through the receiptPersist seam when set
// (tests inject a receipt-CAS failure), otherwise through the production
// state CAS. A failure leaves the row in its source phase without a
// receipt, exactly the crash window the next recovery converges from.
func (a *App) persistTerminalReceipt(op skillstore.Operation) error {
	if a.receiptPersist != nil {
		return a.receiptPersist(op)
	}
	if op.Phase == skillstore.PhaseFinalized {
		return state.MarkOperationFinalized(a.db, op)
	}
	return state.MarkOperationRestored(a.db, op)
}

// cleanupTerminalRow finishes a terminal receipt row whose preparation
// already completed in a previous process: the durable receipt is parsed
// and validated against the operation, the Store resumes or completes the
// evidence cleanup by the receipt-bound identities, and the row is deleted
// by the exact receipt CAS. A foreign, malformed, or mismatched receipt
// preserves the row and blocks recovery.
func (a *App) cleanupTerminalRow(ctx context.Context, op skillstore.Operation) error {
	receipt, err := skillstore.ParseCleanupReceipt(op.Receipt)
	if err != nil {
		return Errorf(CodeRecovery, "operation %d has a malformed terminal receipt", op.ID)
	}
	if err := a.store.CleanupTerminal(ctx, op, receipt); err != nil {
		return err
	}
	return a.deleteOpTerminal(op)
}

// deleteOpTerminal deletes a terminal receipt row through the
// deleteOperation seam when set (tests inject deletion failures), otherwise
// through the exact identity-and-receipt CAS of state.DeleteOperationTerminal.
func (a *App) deleteOpTerminal(op skillstore.Operation) error {
	if a.deleteOperation == nil {
		return state.DeleteOperationTerminal(a.db, op)
	}
	return a.deleteOperation(op.ID)
}

// deleteOpIntent clears a pending or aborted non-terminal intent through
// the deleteOperation seam when set (tests inject deletion failures),
// otherwise through the full-identity/phase CAS of
// state.DeleteOperationIntent. Callers prove the operation evidence absent
// (Store Abort or a proven discard) before clearing the intent.
func (a *App) deleteOpIntent(op skillstore.Operation) error {
	if a.deleteOperation == nil {
		return state.DeleteOperationIntent(a.db, op)
	}
	return a.deleteOperation(op.ID)
}
