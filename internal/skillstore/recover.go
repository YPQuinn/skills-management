package skillstore

import (
	"context"
)

// Restore unwinds a pending (not yet SQLite-committed) operation to the
// pre-operation state as one composite call: the semantic restore
// (PrepareRestore) and the receipt-authorized evidence cleanup
// (CleanupTerminal). It keeps the historical single-call behavior; the
// application boundary uses the two phases so the durable receipt can be
// persisted between them.
func (s Store) Restore(ctx context.Context, op Operation) error {
	receipt, err := s.PrepareRestore(ctx, op)
	if err != nil {
		return err
	}
	if receipt.Empty() {
		// No operation evidence existed at all: the restore is already
		// complete (the never-started or fully-cleaned import state).
		return nil
	}
	op.Phase = PhaseRestored
	return s.CleanupTerminal(ctx, op, receipt)
}

// PrepareRestore performs the semantic unwind of a pending operation and
// returns the durable restore receipt. Both kinds are proven by the
// durable install proof instead of digest matches alone: a surviving staged
// tree proves Install never mutated the live path, and a consumed staged
// tree is only attributable to the operation through the install proof and
// the identity re-proofs described in prepareRestoreImport and
// prepareRestoreReplace. The exact proof/opDir evidence is deliberately
// retained until the durable SQLite receipt persists, so a crash
// immediately before the receipt CAS re-prepares the same terminal state
// and returns the same effective receipt. A zero-value receipt means no
// operation evidence existed at all (a never-started or fully-cleaned
// import restore); the caller may clear the intent directly. Any state
// that cannot be proven is preserved and reported as ErrAmbiguous.
func (s Store) PrepareRestore(ctx context.Context, op Operation) (CleanupReceipt, error) {
	switch op.Kind {
	case KindImport:
		return s.prepareRestoreImport(ctx, op)
	case KindReplace:
		return s.prepareRestoreReplace(ctx, op)
	case KindBaseline:
		return s.prepareRestoreBaseline(ctx, op)
	default:
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "unknown operation kind %q", op.Kind)
	}
}

// Recover applies the deterministic recovery for one unfinished operation:
// a committed operation (the caller decides from SQLite state) is
// finalized, a pending one is restored. The application runs this under the
// Store exclusive lock before any Store write and clears the intent after
// it succeeds; aborted operations are handled by Abort.
func (s Store) Recover(ctx context.Context, op Operation, committed bool) error {
	if committed {
		return s.Finalize(ctx, op)
	}
	return s.Restore(ctx, op)
}
