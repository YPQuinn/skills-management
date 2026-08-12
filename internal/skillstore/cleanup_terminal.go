package skillstore

import (
	"context"
	"errors"
	"os"
	"strconv"
)

// CleanupTerminal removes the receipt-bound operation evidence of a
// finalized or restored row and re-validates the terminal result. The
// receipt format, its action, and its full operation identity are checked
// before any mutation; the terminal expectations (exact physical
// identities, digests, and logical-name bindings) are validated before the
// evidence is touched and again after it is removed, so a byte-identical
// foreign substitution at any point is refused with ErrAmbiguous and every
// candidate is preserved. The evidence removal (terminal_evidence.go) is a
// deterministic state machine over the canonical operation directory, its
// captured-proof slot, and the deterministic terminal slot: every partial
// state left by an interrupted cleanup resumes by the receipt-bound
// identities, and a conflicting canonical/tombstone state, a wrong
// identity, an unexpected child, or a foreign terminal object is preserved.
// When both evidence forms are absent the cleanup is already complete and
// the caller's exact receipt CAS clears the row.
func (s Store) CleanupTerminal(ctx context.Context, op Operation, receipt CleanupReceipt) error {
	if err := validateReceiptBinding(receipt, op); err != nil {
		return errReceiptAmbiguous("terminal receipt does not bind operation %d: %v", op.ID, err)
	}
	switch op.Phase {
	case PhaseFinalized:
		if receipt.action != actionFinalize {
			return errReceiptAmbiguous("receipt action %q does not match the finalized operation %d", receipt.action, op.ID)
		}
	case PhaseRestored:
		if receipt.action != actionRestore {
			return errReceiptAmbiguous("receipt action %q does not match the restored operation %d", receipt.action, op.ID)
		}
	default:
		return errReceiptAmbiguous("operation %d is not a terminal receipt row", op.ID)
	}
	layout, err := s.openLayout()
	if err != nil {
		return errWrap(ErrAmbiguous, "opening the Store layout: %v", err)
	}
	defer layout.close()
	if err := layout.verify(); err != nil {
		return err
	}
	// The terminal result must hold before any evidence is touched.
	if err := s.validateTerminalResult(ctx, layout, op, receipt); err != nil {
		return err
	}
	// The deterministic removal seam fires here, mirroring the strict
	// removal hook of the original finalize, before the strict scan.
	if err := layout.scanTerminalEvidence(op.ID, receipt); err != nil {
		return err
	}
	// The terminal result must still hold at the removal boundary: a
	// byte-identical foreign substitution during the scan is refused before
	// the proof or the operation directory is touched.
	if err := s.validateTerminalResult(ctx, layout, op, receipt); err != nil {
		return err
	}
	if err := layout.removeTerminalEvidence(op.ID, receipt); err != nil {
		return err
	}
	// The terminal result must still hold after the receipt-bound evidence
	// was removed: a byte-identical foreign substitution at the removal
	// boundary is refused before the terminal row may be cleared, so the
	// caller never deletes the receipt row for a result it cannot prove.
	// The layout is re-verified first so the terminal validation never
	// reads through a detached handle.
	if err := layout.verify(); err != nil {
		return err
	}
	return s.validateTerminalResult(ctx, layout, op, receipt)
}

// validateTerminalResult validates the receipt-bound terminal expectations
// against the live Store: exact physical identities, expected digests, and
// logical-name bindings for every terminal object (or the required absence
// of live content after an import restore). Anything else is preserved
// with ErrAmbiguous.
func (s Store) validateTerminalResult(ctx context.Context, layout *storeLayout, op Operation, receipt CleanupReceipt) error {
	switch receipt.action {
	case actionFinalize:
		return s.validateFinalizedResult(ctx, layout, op, receipt)
	case actionRestore:
		return s.validateRestoredResult(ctx, layout, op, receipt)
	}
	return errReceiptAmbiguous("receipt action %q is unknown", receipt.action)
}

// validateFinalizedResult requires the installed live tree, the installed
// Baseline, and (for a replace) the rotated previous snapshot to be the
// exact receipt-bound objects with the expected digests and name bindings:
// a missing object, a byte-identical foreign object, or a corrupt digest
// is preserved with ErrAmbiguous.
func (s Store) validateFinalizedResult(ctx context.Context, layout *storeLayout, op Operation, receipt CleanupReceipt) error {
	live, liveID, liveDigest, err := openTreeDigest(ctx, layout.store, op.Slug)
	if err != nil {
		return errWrap(ErrAmbiguous, "reading live Skill %q: %v", op.Slug, err)
	}
	live.Close()
	if liveID != receipt.liveID {
		return errWrap(ErrAmbiguous, "live Skill %q is not the object %d:%d the receipt records", op.Slug, receipt.liveID.dev, receipt.liveID.ino)
	}
	if liveDigest != op.NewDigest {
		return errWrap(ErrAmbiguous, "live Skill %q does not match the installed digest", op.Slug)
	}
	if err := nameRefersTo(layout.store, op.Slug, liveID); err != nil {
		return errWrap(ErrAmbiguous, "live Skill %q changed while it was being verified: %v", op.Slug, err)
	}
	baseName := strconv.FormatInt(op.SkillID, 10)
	base, baseID, baseDigest, err := openTreeDigest(ctx, layout.baselines, baseName)
	if err != nil {
		return errWrap(ErrAmbiguous, "reading the Baseline of Skill %d: %v", op.SkillID, err)
	}
	base.Close()
	if baseID != receipt.baseID {
		return errWrap(ErrAmbiguous, "the Baseline of Skill %d is not the object %d:%d the receipt records", op.SkillID, receipt.baseID.dev, receipt.baseID.ino)
	}
	if baseDigest != op.NewDigest {
		return errWrap(ErrAmbiguous, "the Baseline of Skill %d does not match the installed digest", op.SkillID)
	}
	if err := nameRefersTo(layout.baselines, baseName, baseID); err != nil {
		return errWrap(ErrAmbiguous, "the Baseline of Skill %d changed while it was being verified: %v", op.SkillID, err)
	}
	if op.Kind == KindReplace {
		prev, prevID, prevDigest, err := openTreeDigest(ctx, layout.previous, baseName)
		if err != nil {
			return errWrap(ErrAmbiguous, "reading the previous snapshot of Skill %d: %v", op.SkillID, err)
		}
		prev.Close()
		if prevID != receipt.prevID {
			return errWrap(ErrAmbiguous, "the previous snapshot of Skill %d is not the object %d:%d the receipt records", op.SkillID, receipt.prevID.dev, receipt.prevID.ino)
		}
		if prevDigest != op.OldDigest {
			return errWrap(ErrAmbiguous, "the previous snapshot of Skill %d does not match the old digest", op.SkillID)
		}
		if err := nameRefersTo(layout.previous, baseName, prevID); err != nil {
			return errWrap(ErrAmbiguous, "the previous snapshot of Skill %d changed while it was being verified: %v", op.SkillID, err)
		}
	}
	return nil
}

// validateRestoredResult requires the terminal restore expectation: for a
// replace the live tree must be the exact receipt-bound recovered object
// with the old digest; for an import any live tree is foreign and blocks.
func (s Store) validateRestoredResult(ctx context.Context, layout *storeLayout, op Operation, receipt CleanupReceipt) error {
	live, liveID, liveDigest, err := openTreeDigest(ctx, layout.store, op.Slug)
	switch {
	case err == nil:
		live.Close()
		if op.Kind == KindImport {
			return errWrap(ErrAmbiguous, "live Skill %q exists after its import restore; it is foreign and preserved", op.Slug)
		}
		if liveID != receipt.liveID || liveDigest != op.OldDigest {
			return errWrap(ErrAmbiguous, "live Skill %q is not the restored object %d:%d the receipt records", op.Slug, receipt.liveID.dev, receipt.liveID.ino)
		}
		return errWrapIf(nameRefersTo(layout.store, op.Slug, liveID),
			"live Skill %q changed while it was being verified: %v", op.Slug)
	case errors.Is(err, os.ErrNotExist):
		if op.Kind == KindReplace {
			return errWrap(ErrAmbiguous, "restored Skill %q is missing", op.Slug)
		}
		return nil
	default:
		return errWrap(ErrAmbiguous, "reading live Skill %q: %v", op.Slug, err)
	}
}

// errWrapIf wraps a non-nil error into ErrAmbiguous; a nil error stays nil.
func errWrapIf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return errWrap(ErrAmbiguous, format, append(args, err)...)
}
