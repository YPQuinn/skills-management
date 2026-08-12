package skillstore

import (
	"context"
	"fmt"
	"os"

	"skillctl/internal/source"
)

// quarantineName is the operation-private slot a pending restore moves a
// live tree into before it is destructively re-proven, so a byte-identical
// unmanaged tree swapped in after the initial proof can never be deleted.
const quarantineName = "live-quarantine"

// verifyQuarantine opens the pinned quarantine exactly once and derives
// its physical identity and canonical digest from that same handle,
// requiring both to match the durable install proof: the quarantine is the
// installed live tree moved by rename, so it must carry the proof's LiveID
// and new digest. The returned root stays open so the caller can re-prove
// the logical name and drain the very object that was verified. A
// byte-identical but independently created directory fails the identity
// check.
func verifyQuarantine(ctx context.Context, opDir *os.Root, proof *installProof) (*os.Root, fileID, error) {
	q, err := openLayoutDir(opDir, quarantineName)
	if err != nil {
		return nil, fileID{}, err
	}
	qID, err := rootID(q)
	if err != nil {
		q.Close()
		return nil, fileID{}, err
	}
	if qID != proof.liveID {
		q.Close()
		return nil, fileID{}, fmt.Errorf("quarantine identity %d:%d does not match the install proof %d:%d",
			qID.dev, qID.ino, proof.liveID.dev, proof.liveID.ino)
	}
	digest, err := source.TreeDigestRoot(ctx, q)
	if err != nil {
		q.Close()
		return nil, fileID{}, err
	}
	if digest != proof.newDigest {
		q.Close()
		return nil, fileID{}, fmt.Errorf("quarantine digest does not match the install proof")
	}
	return q, qID, nil
}

// drainVerifiedQuarantine drains the operation's quarantine after proving
// it is exactly the operation's own install: the quarantine is opened
// exactly once, identity and digest come from that handle, the deterministic
// hook runs between verification and drain, and the logical name must still
// identify the verified object. Immediately before the destructive drain
// the digest is re-verified from the same pinned root: an in-place mutation
// inside the quarantine after the hook (or by a concurrent writer) is
// refused with ErrAmbiguous and preserved, never deleted by identity alone.
// The drain is followed by a final absence re-check: a reappeared quarantine
// is foreign activity and is reported as ErrAmbiguous. A byte-identical
// foreign quarantine swapped after verification survives with ErrAmbiguous.
func (s Store) drainVerifiedQuarantine(ctx context.Context, layout *storeLayout, opDir *os.Root, op Operation, proof *installProof) error {
	q, qID, err := verifyQuarantine(ctx, opDir, proof)
	if err != nil {
		return errWrap(ErrAmbiguous, "operation %d quarantine cannot be proven: %v", op.ID, err)
	}
	defer q.Close()
	// Test seam: a byte-identical foreign quarantine swapped in here must
	// fail the re-proof below and be preserved.
	s.runHook(HookAfterQuarantineOpen)
	if err := nameRefersTo(opDir, quarantineName, qID); err != nil {
		return errWrap(ErrAmbiguous, "operation %d quarantine changed after it was verified; it is preserved", op.ID)
	}
	if err := layout.verify(); err != nil {
		return err
	}
	// Final destructive boundary: the current digest of the already-pinned
	// root must still equal the proof's digest, so content injected into
	// the same directory after the earlier verification is never drained.
	digest, err := source.TreeDigestRoot(ctx, q)
	if err != nil {
		return errWrap(ErrAmbiguous, "operation %d quarantine cannot be re-verified before its drain: %v", op.ID, err)
	}
	if digest != proof.newDigest {
		return errWrap(ErrAmbiguous, "operation %d quarantine changed after it was verified; it is preserved", op.ID)
	}
	if err := drainTree(opDir, quarantineName, qID); err != nil {
		return err
	}
	if exists, err := childExists(opDir, quarantineName); err != nil {
		return err
	} else if exists {
		return errWrap(ErrAmbiguous, "operation %d quarantine reappeared after it was drained", op.ID)
	}
	return nil
}

// drainCandidateIfPresent drains the proof-bound Baseline candidate when it
// still exists; an absent candidate means a previous restore already drained
// it, which is the resume state. The candidate is re-verified at the final
// destructive boundary (identity, logical name, and the proof's digest) so
// content injected into the same directory after the earlier verification
// is never drained.
func drainCandidateIfPresent(ctx context.Context, opDir *os.Root, proof *installProof) error {
	if exists, err := childExists(opDir, "baseline"); err != nil {
		return err
	} else if exists {
		return drainVerifiedTree(ctx, opDir, "baseline", proof.candidateID, proof.newDigest)
	}
	return nil
}
