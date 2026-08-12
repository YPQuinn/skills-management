package skillstore

import (
	"context"
	"errors"
	"os"

	"skillctl/internal/domain"
)

// restoreImport unwinds a pending import to the pre-operation state as one
// composite call: the semantic restore (PrepareRestore) and the
// receipt-authorized evidence cleanup (CleanupTerminal). It keeps the
// historical single-call behavior; the application boundary uses the two
// phases so the durable receipt can be persisted between them.
func (s Store) restoreImport(ctx context.Context, op Operation) error {
	receipt, err := s.prepareRestoreImport(ctx, op)
	if err != nil {
		return err
	}
	op.Phase = PhaseRestored
	return s.CleanupTerminal(ctx, op, receipt)
}

// prepareRestoreImport performs the semantic unwind of a pending import
// and returns the durable restore receipt: a surviving staged tree proves
// Install never mutated the live path and is preserved; a consumed staged
// tree is only attributable to the operation through the durable install
// proof, which binds the live tree, the Baseline candidate, and the
// quarantine. The live tree is only quarantined when its physical identity
// and digest match the proof, and only the verified objects are drained,
// so a byte-identical unmanaged tree swapped in after the proof can never
// be removed. A previous preparation that already drained the candidate or
// the quarantine resumes identically. The exact proof/opDir evidence is
// deliberately retained until the durable SQLite receipt persists; the
// returned receipt binds the operation identity, the opDir and proof
// physical identities, and the terminal expectation that no live tree
// exists after the import restore.
func (s Store) prepareRestoreImport(ctx context.Context, op Operation) (CleanupReceipt, error) {
	if err := domain.ValidateSlug(op.Slug); err != nil {
		return CleanupReceipt{}, err
	}
	layout, err := s.openLayout()
	if err != nil {
		return CleanupReceipt{}, restoreImportNoLayout(ctx, s, op, err)
	}
	defer layout.close()
	s.runHook(HookAfterLayoutOpen)
	if err := layout.verify(); err != nil {
		return CleanupReceipt{}, err
	}
	return s.prepareRestoreImportLayout(ctx, layout, op)
}

// restoreImportNoLayout handles a pending import whose Store layout is
// absent: no operation artifact can exist, so the intent is the not-started
// state. The abort protocol verifies the absence through Store Abort
// without ever touching live content, so a live directory at the slug —
// whether or not its digest matches — is never read, moved, or deleted
// here.
func restoreImportNoLayout(ctx context.Context, s Store, op Operation, openErr error) error {
	if !errors.Is(openErr, os.ErrNotExist) {
		return errWrap(ErrAmbiguous, "opening the Store layout: %v", openErr)
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errWrap(ErrAmbiguous, "opening the Store: %v", err)
	}
	root.Close()
	return nil
}

func (s Store) prepareRestoreImportLayout(ctx context.Context, layout *storeLayout, op Operation) (CleanupReceipt, error) {
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
	if opDir == nil {
		// No operation evidence exists: the operation never started (or a
		// previous restore already cleaned everything). The abort protocol
		// verifies the absence through Store Abort without touching live
		// content, so a live tree at the slug is never read, moved, or
		// deleted here: the Empty state means the intent is clearable, not
		// that the live tree is ours.
		return CleanupReceipt{}, nil
	}
	staged, err := childExists(opDir, "tree")
	if err != nil {
		return CleanupReceipt{}, err
	}
	if staged {
		// A surviving staged tree is only attributable to the operation
		// through a persisted physical proof; on restart the current sample
		// is not proof of ownership, so a byte-identical foreign staging is
		// never drained: the intent and every candidate are preserved with
		// ErrAmbiguous.
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d has surviving staging with no persisted physical proof; it is preserved", op.ID)
	}
	// The staged tree was consumed: the full proof is read before any move
	// or drain and binds every object recovery may touch, and the current
	// operation directory must be the very object the proof records: a
	// byte-identical foreign operation directory carrying a copied proof
	// is refused before any candidate or live tree is touched.
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
	// The live tree and the quarantine state are read before the candidate
	// check: an absent candidate is the resume state only when a previous
	// preparation already drained it, which also drained the quarantine and
	// removed the live tree.
	live, liveID, liveDigest, err := openTreeDigest(ctx, layout.store, op.Slug)
	liveExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "reading live Skill %q: %v", op.Slug, err)
	}
	if liveExists {
		defer live.Close()
	}
	qExists, err := childExists(opDir, quarantineName)
	if err != nil {
		return CleanupReceipt{}, err
	}
	// The Baseline candidate must carry the proof's identity and the new
	// digest when present; an absent candidate is accepted only when the
	// semantic restore already completed (live and quarantine both absent),
	// never while installed content is still present.
	cand, candID, candDigest, err := openTreeDigest(ctx, opDir, "baseline")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d Baseline candidate cannot be read: %v", op.ID, err)
	}
	if err == nil {
		cand.Close()
		if candID != proof.candidateID {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d Baseline candidate is not the object %d:%d the proof records", op.ID, proof.candidateID.dev, proof.candidateID.ino)
		}
		if candDigest != op.NewDigest {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d consumed its staged tree but its Baseline candidate is mismatched", op.ID)
		}
	} else if liveExists || qExists {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d consumed its staged tree but its Baseline candidate is missing or mismatched", op.ID)
	}
	switch {
	case qExists:
		if liveExists {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d has both a live Skill and a quarantine; all candidates are preserved", op.ID)
		}
		if err := s.drainVerifiedQuarantine(ctx, layout, opDir, op, proof); err != nil {
			return CleanupReceipt{}, err
		}
	case !liveExists:
		// The install was already removed (its quarantine cleanup
		// completed); only the operation's own artifacts remain.
	default:
		if liveID != proof.liveID {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "live Skill %q is not the object %d:%d this operation installed", op.Slug, proof.liveID.dev, proof.liveID.ino)
		}
		if liveDigest != proof.newDigest {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "live Skill %q does not match the installed digest", op.Slug)
		}
		// Atomically move the proven live tree into the operation-private
		// quarantine; the destination is opened once and re-proven from
		// that handle before anything is drained, and the move carries the
		// live identity retained from the opened handle above.
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
	if err := drainCandidateIfPresent(ctx, opDir, proof); err != nil {
		return CleanupReceipt{}, err
	}
	return newRestoreReceipt(op, opDirID, proofFileID, proof), nil
}
