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
// committed Baseline path. The candidate in the operation directory must
// carry the proof's CandidateID and the committed digest; when the
// candidate is absent, the installed Baseline is accepted only if its
// physical identity still matches the proof's CandidateID (a byte-identical
// Baseline with a different identity is ErrAmbiguous). During Replace the
// old Baseline is moved aside into baseline-old with the identity retained
// from this invocation's own open, and only that retained identity (or the
// durable witness of a previous preparation) may be drained or rolled back:
// a baseline-old that pre-exists finalization without a witness is
// preserved with ErrAmbiguous. The moved-aside Baseline is drained with its
// retained identity immediately after the candidate is in place, and its
// final absence is re-checked.
func (s Store) installBaseline(ctx context.Context, layout *storeLayout, op Operation, opDir *os.Root, proof *installProof) (backupID fileID, err error) {
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
		if witness.digest != op.OldDigest {
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
	cand, candID, candDigest, err := openTreeDigest(ctx, opDir, "baseline")
	candPresent := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fileID{}, errWrap(ErrAmbiguous, "reading the Baseline candidate of operation %d: %v", op.ID, err)
	}
	if candPresent {
		cand.Close()
		if candID != proof.candidateID {
			return fileID{}, errWrap(ErrAmbiguous, "Baseline candidate for operation %d is not the object %d:%d the proof records", op.ID, proof.candidateID.dev, proof.candidateID.ino)
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
			if baseID != proof.candidateID || baseDigest != op.NewDigest {
				return fileID{}, errWrap(ErrAmbiguous, "the installed Baseline for Skill %d is not the object %d:%d this operation moved", op.SkillID, proof.candidateID.dev, proof.candidateID.ino)
			}
		case baseID == proof.candidateID:
			return fileID{}, errWrap(ErrAmbiguous, "both the operation candidate and the installed Baseline for Skill %d are present", op.SkillID)
		case op.Kind != KindReplace || baseDigest != op.OldDigest:
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
		backupID, err = movePinned(ctx, layout, layout.baselines, baseName, opDir, "baseline-old", baseID, op.OldDigest)
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
		if _, err := movePinned(ctx, layout, opDir, "baseline", layout.baselines, baseName, candID, op.NewDigest); err != nil {
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
					if _, rerr := movePinned(ctx, layout, opDir, "baseline-old", layout.baselines, baseName, backupID, op.OldDigest); rerr != nil {
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
