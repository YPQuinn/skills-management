package skillstore

import (
	"context"
	"errors"
	"os"
	"strconv"

	"skillctl/internal/domain"
)

// restoreReplace unwinds a pending replace to the pre-operation state as
// one composite call: the semantic restore (prepareRestoreReplace) and the
// receipt-authorized evidence cleanup (CleanupTerminal).
func (s Store) restoreReplace(ctx context.Context, op Operation) error {
	receipt, err := s.prepareRestoreReplace(ctx, op)
	if err != nil {
		return err
	}
	op.Phase = PhaseRestored
	return s.CleanupTerminal(ctx, op, receipt)
}

// prepareRestoreReplace performs the semantic unwind of a pending replace
// and returns the durable restore receipt. A surviving staged tree proves
// Install never completed the staged→live rename; a consumed staged tree
// is only attributable to the operation through the durable install
// proof, and the installed live tree is moved into an operation-private
// quarantine and re-proven from its destination handle before it is
// drained and the old content is restored from the proven recovery slot.
// The proof is read in full before any move or drain, the current
// operation directory must be the exact object the proof records (a
// byte-identical foreign operation directory with a copied proof is
// refused), and the proof binds the live tree (LiveID), the Baseline
// candidate (CandidateID), and the recovery slot (RecoveryID). When the
// recovery slot is already gone, the restore resumes only when the live
// tree's physical identity equals the proof's recovery identity with the
// old digest — never from the old digest alone. Every move runs through
// movePinned, so identity always comes from already-open handles. The
// exact proof/opDir evidence is retained until the durable SQLite receipt
// persists. Without any operation evidence a live tree cannot be
// attributed to the operation: even a live tree that still carries the
// old digest is a digest-only match and is preserved with ErrAmbiguous.
func (s Store) prepareRestoreReplace(ctx context.Context, op Operation) (CleanupReceipt, error) {
	if err := domain.ValidateSlug(op.Slug); err != nil {
		return CleanupReceipt{}, err
	}
	layout, err := s.openLayout()
	if err != nil {
		return CleanupReceipt{}, s.restoreReplaceNoLayout(ctx, op, err)
	}
	defer layout.close()
	s.runHook(HookAfterLayoutOpen)
	if err := layout.verify(); err != nil {
		return CleanupReceipt{}, err
	}
	return s.prepareRestoreReplaceLayout(ctx, layout, op)
}

// restoreReplaceNoLayout handles a pending replace whose Store layout is
// absent: no operation artifact can exist, so the intent is the not-started
// state. The abort protocol verifies the absence through Store Abort
// without ever touching live content, so a live tree at the slug — even
// one that still carries the old digest — is never read, moved, or deleted
// here; the Empty state means the intent is clearable, never that the live
// tree is ours.
func (s Store) restoreReplaceNoLayout(ctx context.Context, op Operation, openErr error) error {
	if !errors.Is(openErr, os.ErrNotExist) {
		return errWrap(ErrAmbiguous, "opening the Store layout: %v", openErr)
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errWrap(ErrAmbiguous, "managed Skill %q is missing and its recovery slot is gone", op.Slug)
		}
		return errWrap(ErrAmbiguous, "opening the Store: %v", err)
	}
	root.Close()
	return nil
}

func (s Store) prepareRestoreReplaceLayout(ctx context.Context, layout *storeLayout, op Operation) (CleanupReceipt, error) {
	recName := strconv.FormatInt(op.ID, 10)
	opDir, err := layout.opDir(op.ID)
	switch {
	case err == nil:
		defer opDir.Close()
		if err := layout.verifyOpDir(opDir, op.ID); err != nil {
			return CleanupReceipt{}, err
		}
	case errors.Is(err, os.ErrNotExist):
		opDir = nil
	default:
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "opening operation %d staging: %v", op.ID, err)
	}
	live, liveID, liveDigest, liveExists, err := openRestoreSlot(ctx, layout.store, op.Slug, op.oldUnreadable())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "reading live Skill %q: %v", op.Slug, err)
	}
	if live != nil {
		defer live.Close()
	}
	recRoot, recID, recDigest, recExists, err := openRestoreSlot(ctx, layout.recovery, recName, op.oldUnreadable())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "reading the recovery slot of operation %d: %v", op.ID, err)
	}
	if recRoot != nil {
		recRoot.Close()
	}
	if opDir == nil {
		if !recExists {
			// No operation evidence exists: the operation never started
			// (a replace install always leaves a recovery slot). The abort
			// protocol verifies the absence through Store Abort without
			// touching live content, so the live tree is never read, moved,
			// or deleted here — the Empty state means the intent is
			// clearable, never that the live tree is ours.
			return CleanupReceipt{}, nil
		}
		// A recovery slot without any staging evidence is a state the
		// move-then-install sequence can never produce.
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d has a recovery slot but no staging evidence", op.ID)
	}
	staged, err := childExists(opDir, "tree")
	if err != nil {
		return CleanupReceipt{}, err
	}
	if staged {
		// A surviving staged tree is only attributable to the operation
		// through a persisted physical proof; on restart the current sample
		// is not proof of ownership, so the recovery slot, the live tree,
		// and the staging are all preserved with ErrAmbiguous.
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d has surviving staging with no persisted physical proof; it is preserved", op.ID)
	}
	// The staged tree was consumed: the full proof is read before any move
	// or drain and binds every object recovery may touch, and the current
	// operation directory must be the very object the proof records: a
	// byte-identical foreign operation directory carrying a copied proof
	// is refused before any candidate, live tree, or recovery slot is
	// touched.
	proof, proofFileID, err := readProof(opDir)
	if err != nil || proof == nil {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d consumed its staged tree but has no valid install proof", op.ID)
	}
	if err := validateProof(proof, op); err != nil {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d install proof is invalid: %v", op.ID, err)
	}
	opDirID, err := rootID(opDir)
	if err != nil {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation directory %d cannot be identified: %v", op.ID, err)
	}
	if opDirID != proof.opDirID {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation directory %d is not the object %d:%d the proof records", op.ID, proof.opDirID.dev, proof.opDirID.ino)
	}
	// A previous preparation may have already moved the recovery slot back
	// to live before the receipt persisted; that resume is authorized only
	// by the live tree's physical identity matching the proof's recovery
	// identity with the old digest, never by the old digest alone.
	alreadyRestored := !recExists && liveExists && liveID == proof.recoveryID && liveDigest == op.OldDigest
	if !recExists && !alreadyRestored {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d has no recovery slot and its live Skill cannot be attributed to the restore", op.ID)
	}
	// The Baseline candidate must carry the proof's identity and the new
	// digest when present; an absent candidate is the resume state only
	// when a previous preparation already drained it (the recovery slot is
	// gone and the live tree is the restored object). A keep-baseline
	// operation (rollback) never creates a candidate, so any present
	// candidate is foreign and preserved.
	cand, candID, candDigest, err := openTreeDigest(ctx, opDir, "baseline")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d Baseline candidate cannot be read: %v", op.ID, err)
	}
	candPresent := err == nil
	if candPresent {
		cand.Close()
	}
	switch {
	case op.BaselineMode == BaselineKeep && candPresent:
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "keep-baseline operation %d carries an unexpected Baseline candidate; it is preserved", op.ID)
	case op.BaselineMode != BaselineKeep && candPresent:
		if candID != proof.candidateID {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d Baseline candidate is not the object %d:%d the proof records", op.ID, proof.candidateID.dev, proof.candidateID.ino)
		}
		if candDigest != op.NewDigest {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d consumed its staged tree but its Baseline candidate is mismatched", op.ID)
		}
	case op.BaselineMode != BaselineKeep && !candPresent && !alreadyRestored:
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d consumed its staged tree but its Baseline candidate is missing or mismatched", op.ID)
	}
	qExists, err := childExists(opDir, quarantineName)
	if err != nil {
		return CleanupReceipt{}, err
	}
	switch {
	case qExists:
		if alreadyRestored || liveExists {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d has both a live Skill and a quarantine; all candidates are preserved", op.ID)
		}
		if err := s.drainVerifiedQuarantine(ctx, layout, opDir, op, proof); err != nil {
			return CleanupReceipt{}, err
		}
	case alreadyRestored:
		// The recovery slot is gone and the live tree is the exact object
		// this operation moved into recovery: the previous restore already
		// completed the semantic unwind, so only the receipt and the
		// evidence removal remain.
	case !liveExists:
		// No installed content to quarantine; the recovery-slot restore
		// below completes the unwind.
	default:
		if liveID != proof.liveID {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "live Skill %q is not the object %d:%d this operation installed", op.Slug, proof.liveID.dev, proof.liveID.ino)
		}
		if liveDigest != proof.newDigest {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "live Skill %q does not match the installed digest", op.Slug)
		}
		if _, err := movePinned(ctx, layout, layout.store, op.Slug, opDir, quarantineName, liveID, proof.newDigest); err != nil {
			if errors.Is(err, errDestinationExists) {
				return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d quarantine already exists", op.ID)
			}
			return CleanupReceipt{}, err
		}
		if err := s.drainVerifiedQuarantine(ctx, layout, opDir, op, proof); err != nil {
			return CleanupReceipt{}, err
		}
	}
	if recExists {
		if recID != proof.recoveryID || recDigest != op.OldDigest {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d installed content but its recovery slot is missing or mismatched", op.ID)
		}
		// Restore the old content from the proven recovery slot; a
		// non-tree object moves by identity only (no digest exists).
		if op.oldUnreadable() {
			if err := moveNonTree(layout, layout.recovery, recName, layout.store, op.Slug, recID); err != nil {
				if errors.Is(err, errDestinationExists) {
					return CleanupReceipt{}, errWrap(ErrAmbiguous, "live Skill %q appeared while recovery was restoring it", op.Slug)
				}
				return CleanupReceipt{}, err
			}
		} else {
			if _, err := movePinned(ctx, layout, layout.recovery, recName, layout.store, op.Slug, recID, op.OldDigest); err != nil {
				if errors.Is(err, errDestinationExists) {
					return CleanupReceipt{}, errWrap(ErrAmbiguous, "live Skill %q appeared while recovery was restoring it", op.Slug)
				}
				return CleanupReceipt{}, err
			}
		}
	}
	if err := drainCandidateIfPresent(ctx, opDir, proof); err != nil {
		return CleanupReceipt{}, err
	}
	return newRestoreReceipt(op, opDirID, proofFileID, proof), nil
}

// openRestoreSlot opens one replace-restore participant: a digestable
// tree, or — for an unreadable-old operation — any node, opened by
// identity only when it is not a directory. A non-directory carries an
// empty digest that only matches an empty operation old digest.
func openRestoreSlot(ctx context.Context, parent *os.Root, name string, unreadable bool) (root *os.Root, id fileID, digest string, exists bool, err error) {
	if !unreadable {
		root, id, digest, err = openTreeDigest(ctx, parent, name)
		return root, id, digest, err == nil, err
	}
	info, lerr := parent.Lstat(name)
	switch {
	case errors.Is(lerr, os.ErrNotExist):
		return nil, fileID{}, "", false, nil
	case lerr != nil:
		return nil, fileID{}, "", false, lerr
	case info.IsDir():
		root, id, digest, err = openTreeDigest(ctx, parent, name)
		return root, id, digest, err == nil, err
	default:
		id, err = fileIDOf(info)
		return nil, id, "", err == nil, err
	}
}
