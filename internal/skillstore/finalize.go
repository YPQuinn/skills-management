package skillstore

import (
	"context"
	"errors"
	"os"
	"strconv"

	"skillctl/internal/domain"
)

// Finalize completes a committed operation as one composite call: the
// semantic finalization (PrepareFinalize) and the receipt-authorized
// evidence cleanup (CleanupTerminal). It keeps the historical single-call
// behavior; the application boundary uses the two phases so the durable
// receipt can be persisted between them.
func (s Store) Finalize(ctx context.Context, op Operation) error {
	receipt, err := s.PrepareFinalize(ctx, op)
	if err != nil {
		return err
	}
	op.Phase = PhaseFinalized
	return s.CleanupTerminal(ctx, op, receipt)
}

// PrepareFinalize performs the semantic terminal mutation of a committed
// operation under the durable install proof: the original operation
// directory must exist and its proof must be valid, the live tree is bound
// to the proof's LiveID and the committed digest, the digest-proven
// Baseline candidate moves into .skillctl/baselines/<skillID>, a replace
// rotates the proven recovery slot into the single previous snapshot at
// .skillctl/previous/<skillID>, and every transient moved by this
// invocation is drained with its retained identity. The moved-aside
// Baseline and stale previous snapshot are recorded in a durable witness
// before they move, so a crash between a move and its drain resumes on the
// next preparation by the exact witnessed identity. The layout is
// re-verified and the live tree is bound again after every phase
// succeeded. The exact proof/opDir evidence is deliberately retained until
// the durable SQLite receipt persists, and the returned receipt binds the
// operation identity, the opDir and proof physical identities, and the
// terminal-result expectations (installed live, Baseline, and rotated
// previous objects) so a byte-identical foreign replacement is refused
// during the later cleanup. Preparation is idempotent: a crash immediately
// before the SQL receipt persistence re-prepares the same terminal state
// and returns the same effective receipt.
func (s Store) PrepareFinalize(ctx context.Context, op Operation) (CleanupReceipt, error) {
	if op.SkillID == 0 {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d has no committed Skill id", op.ID)
	}
	if op.Kind != KindImport && op.Kind != KindReplace && op.Kind != KindBaseline {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "unknown operation kind %q", op.Kind)
	}
	if err := domain.ValidateSlug(op.Slug); err != nil {
		return CleanupReceipt{}, err
	}
	if op.Kind == KindBaseline {
		return s.prepareFinalizeBaseline(ctx, op)
	}
	layout, err := s.openLayout()
	if err != nil {
		return CleanupReceipt{}, err
	}
	defer layout.close()
	s.runHook(HookAfterLayoutOpen)
	if err := layout.verify(); err != nil {
		return CleanupReceipt{}, err
	}
	opDir, err := layout.opDir(op.ID)
	if errors.Is(err, os.ErrNotExist) {
		// A committed operation whose staging is gone cannot be attributed;
		// the directory is never recreated.
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation directory %d is missing after its commit; it is not recreated", op.ID)
	}
	if err != nil {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "opening operation %d staging: %v", op.ID, err)
	}
	defer opDir.Close()
	if err := layout.verifyOpDir(opDir, op.ID); err != nil {
		return CleanupReceipt{}, err
	}
	proof, proofFileID, err := readProof(opDir)
	if err != nil || proof == nil {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d has no valid install proof: %v", op.ID, err)
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
	// The live tree is opened once and must carry the installed identity
	// and the committed digest; a byte-identical foreign live tree is
	// refused by identity even though its digest matches.
	if err := verifyLiveBound(ctx, layout, op, proof); err != nil {
		return CleanupReceipt{}, err
	}
	var backupID fileID
	var receiptBaseID fileID
	if op.BaselineMode == BaselineKeep {
		// A rollback retains the pre-operation Baseline: the receipt binds
		// the identity sampled from the current Baseline so the terminal
		// validation can refuse a swapped slot, while the content is never
		// required to match the operation's digest (the Baseline was never
		// mutated, and its bytes are comparison state rather than live
		// content).
		base, id, _, err := openTreeDigest(ctx, layout.baselines, strconv.FormatInt(op.SkillID, 10))
		if err != nil {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "reading the retained Baseline of Skill %d: %v", op.SkillID, err)
		}
		base.Close()
		receiptBaseID = id
	} else {
		backupID, err = s.installBaseline(ctx, layout, op, opDir, proof)
		if err != nil {
			return CleanupReceipt{}, err
		}
		receiptBaseID = proof.candidateID
	}
	var staleID fileID
	var staleWitness *transientWitness
	if op.Kind == KindReplace {
		if op.oldUnreadable() {
			// The displaced non-tree content can never be a previous
			// snapshot: the finalize removes it by identity.
			if err := s.removeRecoveryNonTree(layout, op, proof); err != nil {
				return CleanupReceipt{}, err
			}
		} else {
			staleID, staleWitness, err = s.rotatePrevious(ctx, layout, op, proof, opDir)
			if err != nil {
				return CleanupReceipt{}, err
			}
		}
	}
	// Every phase succeeded; the transients moved by this invocation are
	// drained with their retained identities (or the durable witness of a
	// previous preparation), their final absence is re-checked, and their
	// witnesses are removed — a reappeared transient is foreign activity.
	if backupID != (fileID{}) {
		layout.runHook(HookBeforeTransientDrain)
		// Final destructive boundary: the moved-aside Baseline must still
		// carry the old digest the Baseline slot and its witness bind.
		if err := drainVerifiedTree(ctx, opDir, "baseline-old", backupID, op.displacedBaselineDigest()); err != nil {
			return CleanupReceipt{}, err
		}
		layout.runHook(HookAfterTransientDrain)
		if exists, err := childExists(opDir, "baseline-old"); err != nil {
			return CleanupReceipt{}, err
		} else if exists {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "the saved Baseline for operation %d reappeared after it was drained", op.ID)
		}
		if err := removeWitnessIfPresent(opDir, baselineWitnessName, op.ID, witnessBaseline); err != nil {
			return CleanupReceipt{}, err
		}
	}
	if staleID != (fileID{}) {
		layout.runHook(HookBeforeTransientDrain)
		staleName := strconv.FormatInt(op.SkillID, 10) + ".old"
		// Final destructive boundary: the stale snapshot must still carry
		// the digest its witness records.
		if err := drainVerifiedTree(ctx, layout.previous, staleName, staleID, staleWitness.digest); err != nil {
			return CleanupReceipt{}, err
		}
		layout.runHook(HookAfterTransientDrain)
		if exists, err := childExists(layout.previous, staleName); err != nil {
			return CleanupReceipt{}, err
		} else if exists {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "the stale previous snapshot for Skill %d reappeared after it was drained", op.SkillID)
		}
		if err := removeWitnessIfPresent(opDir, staleWitnessName, op.ID, witnessStalePrev); err != nil {
			return CleanupReceipt{}, err
		}
	}
	// The operation directory now holds only the proof file. Before the
	// receipt is returned, the layout is re-verified and the live tree is
	// bound again: it must still be the object this operation installed,
	// with the committed digest and the logical name still identifying it.
	// A byte-identical foreign live tree swapped in after the first proof
	// is refused here with ErrAmbiguous: the operation directory and its
	// proof stay in place for the receipt-bound cleanup, the Baseline and
	// previous changes made by the proven phases are retained, and the
	// foreign live tree is preserved.
	if err := layout.verify(); err != nil {
		return CleanupReceipt{}, err
	}
	if err := verifyLiveBound(ctx, layout, op, proof); err != nil {
		return CleanupReceipt{}, err
	}
	return newFinalizeReceipt(op, opDirID, proofFileID, proof, receiptBaseID), nil
}

// removeRecoveryNonTree removes the recovery object of an unreadable-old
// replace by identity (not a Skill tree, so no snapshot rotation); an
// absent slot resumes.
func (s Store) removeRecoveryNonTree(layout *storeLayout, op Operation, proof *installProof) error {
	recName := strconv.FormatInt(op.ID, 10)
	if _, err := layout.recovery.Lstat(recName); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return errWrap(ErrAmbiguous, "reading the recovery slot of operation %d: %v", op.ID, err)
	}
	s.runHook(HookBeforeTransientDrain)
	if err := proveNonTree(layout.recovery, recName, proof.recoveryID); err != nil {
		return err
	}
	if err := layout.recovery.Remove(recName); err != nil {
		return errWrap(ErrAmbiguous, "recovery slot of operation %d could not be removed: %v", op.ID, err)
	}
	if exists, err := childExists(layout.recovery, recName); err != nil {
		return err
	} else if exists {
		return errWrap(ErrAmbiguous, "recovery slot of operation %d reappeared after its removal", op.ID)
	}
	return nil
}

// verifyLiveBound opens the live Skill and requires it to carry the
// install proof's LiveID and the committed digest, with the logical name
// still identifying the opened object. It is the final authority before
// Finalize removes the operation directory or reports success: a
// byte-identical foreign live tree is refused by physical identity even
// though its digest matches, and every candidate is preserved.
func verifyLiveBound(ctx context.Context, layout *storeLayout, op Operation, proof *installProof) error {
	live, liveID, liveDigest, err := openTreeDigest(ctx, layout.store, op.Slug)
	if err != nil {
		return errWrap(ErrAmbiguous, "reading live Skill %q: %v", op.Slug, err)
	}
	defer live.Close()
	if liveID != proof.liveID {
		return errWrap(ErrAmbiguous, "live Skill %q is not the object %d:%d this operation installed", op.Slug, proof.liveID.dev, proof.liveID.ino)
	}
	if liveDigest != op.NewDigest {
		return errWrap(ErrAmbiguous, "live Skill %q digest %s does not match the committed digest %s", op.Slug, liveDigest, op.NewDigest)
	}
	if err := nameRefersTo(layout.store, op.Slug, liveID); err != nil {
		return errWrap(ErrAmbiguous, "live Skill %q changed while it was being verified: %v", op.Slug, err)
	}
	return nil
}
