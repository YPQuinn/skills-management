package skillstore

import (
	"context"
	"errors"
	"os"
	"strconv"
)

// baselineWitnessName is the durable progress witness of the finalize
// Baseline backup: the physical identity and digest of the pre-replace
// Baseline a preparation invocation moved aside into baseline-old. It is
// written before the move and removed after the backup is drained, so a
// crash between the move and the drain leaves the fresh process able to
// attribute the backup to this operation; any other content in the
// baseline-old slot is foreign and preserved.
const baselineWitnessName = "baseline-backup-witness"

// installBaseline moves the proof-bound operation candidate into the
// committed Baseline path for an advancing replace or import.
func (s Store) installBaseline(ctx context.Context, layout *storeLayout, op Operation, opDir *os.Root, proof *installProof) (fileID, error) {
	return s.installBaselineFrom(ctx, layout, op, opDir, proof.candidateID, "baseline")
}

// installBaselineFrom moves one candidate tree (candName carrying candID
// and the committed digest) into the committed Baseline path, idempotently.
// A replace/import moves its copy at opDir/baseline; a Baseline-only
// refresh moves the staged tree itself. An absent candidate is accepted
// only when the installed Baseline is the moved candidate itself. The old
// Baseline is moved aside into baseline-old with the identity retained from
// this invocation's own open (or the durable witness of a previous
// preparation); a baseline-old without a witness is preserved. The expected
// pre-operation Baseline digest is the intent's displaced digest: the
// recorded BaselineDigest, or the displaced live digest — so an Accept
// Source repair of a missing Store tree advances the recorded Baseline
// through the same journal.
func (s Store) installBaselineFrom(ctx context.Context, layout *storeLayout, op Operation, opDir *os.Root, candID fileID, candName string) (backupID fileID, err error) {
	baseName := strconv.FormatInt(op.SkillID, 10)
	witness, err := resolveWitnessState(opDir, baselineWitnessName, op.ID, witnessBaseline)
	if err != nil {
		return fileID{}, errWrap(ErrAmbiguous, "operation %d baseline witness cannot be resolved: %v", op.ID, err)
	}
	backupExists, err := childExists(opDir, "baseline-old")
	if err != nil {
		return fileID{}, err
	}
	switch {
	case backupExists && witness == nil:
		return fileID{}, errWrap(ErrAmbiguous, "a saved Baseline already exists for operation %d before finalization; it is preserved", op.ID)
	case backupExists:
		// A previous preparation crashed after moving the old Baseline
		// aside: the backup is attributed only by the durable witness, and
		// only while it is still the exact witnessed object with the old
		// digest the Baseline slot requires.
		if witness.digest != op.displacedBaselineDigest() {
			return fileID{}, errWrap(ErrAmbiguous, "saved Baseline for operation %d has a witness digest that does not match the old digest; it is preserved", op.ID)
		}
		if err := verifyWitnessTree(ctx, opDir, "baseline-old", witness); err != nil {
			return fileID{}, errWrap(ErrAmbiguous, "saved Baseline for operation %d cannot be attributed to its witness; it is preserved: %v", op.ID, err)
		}
		backupID = witness.id
	case witness != nil:
		// The backup was already drained by a previous preparation; the
		// stale witness is removed.
		if err := removeWitnessFile(opDir, baselineWitnessName, witness); err != nil {
			return fileID{}, err
		}
	}
	// The candidate must be the proof's object; an absent candidate is only
	// acceptable when the installed Baseline is the moved candidate itself.
	cand, candActualID, candDigest, err := openTreeDigest(ctx, opDir, candName)
	candPresent := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fileID{}, errWrap(ErrAmbiguous, "reading the Baseline candidate of operation %d: %v", op.ID, err)
	}
	if candPresent {
		cand.Close()
		if candActualID != candID {
			return fileID{}, errWrap(ErrAmbiguous, "Baseline candidate for operation %d is not the object %d:%d the proof records", op.ID, candID.dev, candID.ino)
		}
		if candDigest != op.NewDigest {
			return fileID{}, errWrap(ErrAmbiguous, "Baseline candidate for operation %d does not match the committed digest", op.ID)
		}
	}
	base, baseID, baseDigest, err := openTreeDigest(ctx, layout.baselines, baseName)
	baseExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fileID{}, errWrap(ErrAmbiguous, "reading the Baseline of Skill %d: %v", op.SkillID, err)
	}
	if baseExists {
		base.Close()
		switch {
		case !candPresent:
			// The candidate was already moved into place: the installed
			// Baseline must be the moved candidate itself.
			if baseID != candID || baseDigest != op.NewDigest {
				return fileID{}, errWrap(ErrAmbiguous, "the installed Baseline for Skill %d is not the object %d:%d this operation moved", op.SkillID, candID.dev, candID.ino)
			}
		case baseID == candID:
			return fileID{}, errWrap(ErrAmbiguous, "both the operation candidate and the installed Baseline for Skill %d are present", op.SkillID)
		case baseDigest != op.displacedBaselineDigest():
			return fileID{}, errWrap(ErrAmbiguous, "existing Baseline for Skill %d has unexpected digest %s", op.SkillID, baseDigest)
		}
	} else if !candPresent {
		return fileID{}, errWrap(ErrAmbiguous, "operation %d candidate is missing and no Baseline can be attributed to it", op.ID)
	}
	// The old Baseline must move aside before the candidate can take its
	// name; the witness is written before the move so a crash between the
	// move and the drain can still attribute the backup. When the candidate
	// is already in place (base == candidateID) no move is needed. On a
	// resume the backup is already witnessed and only the candidate move
	// remains.
	if candPresent && baseExists && backupID == (fileID{}) {
		// Test seam: a byte-identical foreign Baseline swapped in here must
		// be refused by the source identity re-proof of the first move
		// below.
		s.runHook(HookBeforeBaselineMoves)
		witness, err = writeTransientWitness(opDir, baselineWitnessName, op.ID, witnessBaseline, baseID, baseDigest)
		if err != nil {
			return fileID{}, errWrap(ErrAmbiguous, "operation %d baseline witness could not be written: %v", op.ID, err)
		}
		backupID, err = movePinned(ctx, layout, layout.baselines, baseName, opDir, "baseline-old", baseID, op.displacedBaselineDigest())
		if err != nil {
			if errors.Is(err, errDestinationExists) {
				return fileID{}, errWrap(ErrAmbiguous, "a saved Baseline appeared for operation %d", op.ID)
			}
			return fileID{}, err
		}
	} else if candPresent {
		s.runHook(HookBeforeBaselineMoves)
	}
	if candPresent {
		if _, err := movePinned(ctx, layout, opDir, candName, layout.baselines, baseName, candID, op.NewDigest); err != nil {
			if errors.Is(err, errDestinationExists) {
				return fileID{}, errWrap(ErrAmbiguous, "a Baseline appeared while operation %d was being finalized", op.ID)
			}
			// Roll back through the same safe move, re-proving the saved
			// Baseline from its destination handle. backupID is never zero
			// here: a pre-existing saved Baseline is refused above, so only
			// a Baseline this invocation moved (or attributed through the
			// witness) can be returned.
			if backupID != (fileID{}) {
				if _, statErr := layout.baselines.Lstat(baseName); os.IsNotExist(statErr) {
					if _, rerr := movePinned(ctx, layout, opDir, "baseline-old", layout.baselines, baseName, backupID, op.displacedBaselineDigest()); rerr != nil {
						return fileID{}, errWrap(ErrAmbiguous, "saved Baseline could not be returned to %q: %v", baseName, rerr)
					}
					if rerr := removeWitnessFile(opDir, baselineWitnessName, witness); rerr != nil {
						return fileID{}, err
					}
					backupID = fileID{}
				}
			}
			return fileID{}, err
		}
	}
	// The moved-aside Baseline is drained only with the identity retained
	// from this invocation's move (or the witness); the caller drains it
	// after every phase succeeded, so a failed rotation preserves it for
	// deterministic recovery.
	return backupID, nil
}

// prepareFinalizeBaseline completes a committed Baseline-only refresh: the
// staged tree's identity is recorded in the durable install proof before
// the swap, the staged tree becomes the new Baseline under the durable
// witness, the displaced Baseline is drained, and the receipt binds the
// installed Baseline and the untouched live tree. Preparation is
// idempotent: a crash after the swap re-prepares by the proof-bound
// identities and returns the same effective receipt.
func (s Store) prepareFinalizeBaseline(ctx context.Context, op Operation) (CleanupReceipt, error) {
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
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation directory %d is missing after its commit; it is not recreated", op.ID)
	}
	if err != nil {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "opening operation %d staging: %v", op.ID, err)
	}
	defer opDir.Close()
	if err := layout.verifyOpDir(opDir, op.ID); err != nil {
		return CleanupReceipt{}, err
	}
	// The staged tree is the Baseline candidate: its identity is recorded
	// in the durable proof before the swap, so an interrupted swap resumes
	// by the proof-bound identities.
	proof, proofFileID, err := readProof(opDir)
	if err != nil {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation %d install proof cannot be read: %v", op.ID, err)
	}
	if proof == nil {
		tree, treeID, digest, err := openTreeDigest(ctx, opDir, "tree")
		if err != nil {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "reading the Baseline candidate of operation %d: %v", op.ID, err)
		}
		tree.Close()
		if digest != op.NewDigest {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "Baseline candidate for operation %d does not match the committed digest", op.ID)
		}
		if err := nameRefersTo(opDir, "tree", treeID); err != nil {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "Baseline candidate for operation %d changed after it was verified: %v", op.ID, err)
		}
		opDirID, err := rootID(opDir)
		if err != nil {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "operation directory %d cannot be identified: %v", op.ID, err)
		}
		if err := writeProof(opDir, installProof{
			opID: op.ID, kind: op.Kind, slug: op.Slug,
			oldDigest: op.OldDigest, newDigest: op.NewDigest,
			opDirID: opDirID, candidateID: treeID,
		}); err != nil {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "Baseline install proof of operation %d could not be written: %v", op.ID, err)
		}
		proof, proofFileID, err = readProof(opDir)
		if err != nil || proof == nil {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "Baseline install proof of operation %d cannot be re-read: %v", op.ID, err)
		}
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
	// The live tree is never mutated: only its untouched identity is
	// sampled for the terminal receipt.
	live, liveID, _, err := openTreeDigest(ctx, layout.store, op.Slug)
	if err != nil {
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "reading live Skill %q: %v", op.Slug, err)
	}
	live.Close()
	backupID, err := s.installBaselineFrom(ctx, layout, op, opDir, proof.candidateID, "tree")
	if err != nil {
		return CleanupReceipt{}, err
	}
	if backupID != (fileID{}) {
		layout.runHook(HookBeforeTransientDrain)
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
	if err := layout.verify(); err != nil {
		return CleanupReceipt{}, err
	}
	return CleanupReceipt{
		action: actionFinalize, op: op,
		opDirID: opDirID, proofID: proofFileID,
		liveID: liveID, baseID: proof.candidateID,
	}, nil
}
