package skillstore

import (
	"context"
	"errors"
	"os"
	"strconv"
)

// staleWitnessName is the durable progress witness of the finalize
// previous-snapshot rotation: the physical identity and digest of the
// superseded previous snapshot a preparation invocation moved aside into
// <skillID>.old. It is written before the move and removed after the stale
// snapshot is drained, so a crash between the move and the drain leaves
// the fresh process able to attribute the stale snapshot to this
// operation; any other content in the stale slot is foreign and preserved.
const staleWitnessName = "stale-previous-witness"

// rotatePrevious installs the operation's proof-bound recovery slot as the
// Skill's single previous snapshot. A recovery slot that exists must carry
// the proof's RecoveryID and the old digest; when the recovery slot is
// absent, the previous snapshot is accepted only if its physical identity
// still matches the proof's RecoveryID (already rotated). A stale snapshot
// that pre-exists rotation without a witness has no operation-attribution
// proof, so even a byte-identical candidate is preserved with ErrAmbiguous;
// a witnessed stale snapshot resumes. The superseded previous snapshot is
// moved aside with the identity retained from this invocation's own open
// (or the witness), the recovery content is renamed into place with its
// retained identity, and only the stale snapshot moved by this invocation
// is drained, with its retained identity and a final absence re-check. The
// retained stale identity is returned as cleanup evidence.
func (s Store) rotatePrevious(ctx context.Context, layout *storeLayout, op Operation, proof *installProof, opDir *os.Root) (staleID fileID, witness *transientWitness, err error) {
	prevName := strconv.FormatInt(op.SkillID, 10)
	staleName := prevName + ".old"
	recName := strconv.FormatInt(op.ID, 10)
	witness, err = resolveWitnessState(opDir, staleWitnessName, op.ID, witnessStalePrev)
	if err != nil {
		return fileID{}, nil, errWrap(ErrAmbiguous, "operation %d previous witness cannot be resolved: %v", op.ID, err)
	}
	staleExists, err := childExists(layout.previous, staleName)
	if err != nil {
		return fileID{}, nil, err
	}
	switch {
	case staleExists && witness == nil:
		return fileID{}, nil, errWrap(ErrAmbiguous, "stale previous snapshot %q pre-existed rotation; it is preserved", staleName)
	case staleExists:
		// A previous preparation crashed after moving the superseded
		// snapshot aside: the stale snapshot is attributed only by the
		// durable witness, and only while it is still the exact witnessed
		// object.
		if err := verifyWitnessTree(ctx, layout.previous, staleName, witness); err != nil {
			return fileID{}, nil, errWrap(ErrAmbiguous, "stale previous snapshot %q cannot be attributed to its witness; it is preserved: %v", staleName, err)
		}
		staleID = witness.id
	case witness != nil:
		// The stale snapshot was already drained by a previous
		// preparation; the stale witness is removed.
		if err := removeWitnessFile(opDir, staleWitnessName, witness); err != nil {
			return fileID{}, nil, err
		}
		witness = nil
	}
	prev, prevID, prevDigest, err := openTreeDigest(ctx, layout.previous, prevName)
	prevExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fileID{}, nil, errWrap(ErrAmbiguous, "reading the previous snapshot of Skill %d: %v", op.SkillID, err)
	}
	if prevExists {
		prev.Close()
	}
	rec, recID, recDigest, err := openTreeDigest(ctx, layout.recovery, recName)
	recExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fileID{}, nil, errWrap(ErrAmbiguous, "reading the recovery slot of operation %d: %v", op.ID, err)
	}
	if recExists {
		rec.Close()
	}
	if !recExists {
		// The recovery slot is gone: the rotation is recognized only when
		// the installed previous snapshot is the moved recovery object (a
		// witnessed stale snapshot still needs its drain below).
		if !prevExists || prevID != proof.recoveryID || prevDigest != op.OldDigest {
			return fileID{}, nil, errWrap(ErrAmbiguous, "recovery slot for operation %d is missing and the previous snapshot cannot be attributed to it", op.ID)
		}
	} else {
		if recID != proof.recoveryID {
			return fileID{}, nil, errWrap(ErrAmbiguous, "recovery slot for operation %d is not the object %d:%d the proof records", op.ID, proof.recoveryID.dev, proof.recoveryID.ino)
		}
		if recDigest != op.OldDigest {
			return fileID{}, nil, errWrap(ErrAmbiguous, "recovery slot for operation %d does not match the old digest", op.ID)
		}
		if prevExists && prevID == proof.recoveryID {
			return fileID{}, nil, errWrap(ErrAmbiguous, "recovery slot for operation %d duplicates the installed previous snapshot", op.ID)
		}
		if prevExists && staleID == (fileID{}) {
			// The superseded previous snapshot must move aside before the
			// recovery content can take its name; the witness is written
			// before the move so a crash between the move and the drain
			// can still attribute the stale snapshot.
			witness, err = writeTransientWitness(opDir, staleWitnessName, op.ID, witnessStalePrev, prevID, prevDigest)
			if err != nil {
				return fileID{}, nil, errWrap(ErrAmbiguous, "operation %d previous witness could not be written: %v", op.ID, err)
			}
			staleID, err = movePinned(ctx, layout, layout.previous, prevName, layout.previous, staleName, prevID, prevDigest)
			if err != nil {
				if errors.Is(err, errDestinationExists) {
					return fileID{}, nil, errWrap(ErrAmbiguous, "a saved previous snapshot appeared for Skill %d", op.SkillID)
				}
				return fileID{}, nil, err
			}
		}
		if _, err := movePinned(ctx, layout, layout.recovery, recName, layout.previous, prevName, recID, op.OldDigest); err != nil {
			if errors.Is(err, errDestinationExists) {
				if staleID != (fileID{}) {
					// Safe rollback: the moved previous snapshot returns to
					// its slot with its retained identity re-proven.
					if _, rerr := movePinned(ctx, layout, layout.previous, staleName, layout.previous, prevName, staleID, prevDigest); rerr != nil {
						return fileID{}, nil, errWrap(ErrAmbiguous, "previous snapshot could not be returned to %q: %v", prevName, rerr)
					}
					if rerr := removeWitnessFile(opDir, staleWitnessName, witness); rerr != nil {
						return fileID{}, nil, err
					}
					witness = nil
					staleID = fileID{}
				}
				return fileID{}, nil, errWrap(ErrAmbiguous, "a previous snapshot appeared for Skill %d during rotation", op.SkillID)
			}
			return fileID{}, nil, err
		}
	}
	// The stale snapshot is drained only with the identity retained from
	// this invocation's move (or the witness); the caller drains it after
	// every phase succeeded, so a failed rotation preserves it for
	// deterministic recovery. The witness travels with the stale identity
	// so the caller can re-verify the digest at the drain boundary.
	return staleID, witness, nil
}
