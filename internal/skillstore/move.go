package skillstore

import (
	"context"
	"errors"
	"os"

	"skillctl/internal/source"
)

// movePinned moves the single-component tree oldName below oldParent onto
// newName below newParent with a no-replace rename, and proves the moved
// tree from its destination handle. The caller passes the physical identity
// it sampled from its own already-open handle (wantID) together with the
// digest it hashed from that handle (wantDigest): movePinned never
// re-authorizes a source it did not see hashed, so a byte-identical foreign
// tree swapped into the source slot after the caller's proof is refused
// before anything moves. Before the rename the source must still carry the
// caller's identity; after the rename the destination is opened exactly
// once and its identity and canonical digest must match the retained proof,
// and the destination name is re-proven once more after verification. Both
// parents are synced and the logical layout is revalidated before and
// after. Every failure after the forward rename is ErrAmbiguous.
//
// When the destination open, digest, or name proof fails, the reverse
// rollback is authorized only by a fresh re-open of the destination that
// still carries the caller's original identity and digest: the first
// destination sample is never rollback authority, so a byte-identical
// foreign destination swapped in after the rename is preserved in place
// with ErrAmbiguous. The rollback itself is a full re-proven move
// (no-replace rename, parent syncs, and identity/digest/name verification
// of the restored tree from its opened handle) followed by a layout
// re-validation, and the source slot must be empty for it. Anything that
// cannot be proven is preserved and reported as ErrAmbiguous.
func movePinned(ctx context.Context, layout *storeLayout, oldParent *os.Root, oldName string, newParent *os.Root, newName string, wantID fileID, wantDigest string) (fileID, error) {
	if err := layout.verify(); err != nil {
		return fileID{}, err
	}
	src, srcID, err := openPinnedChild(oldParent, oldName)
	if err != nil {
		return fileID{}, err
	}
	defer src.Close()
	// The source must still be the exact object the caller hashed and
	// identified; a swapped source is never certified as the caller's.
	if srcID != wantID {
		return fileID{}, errWrap(ErrAmbiguous, "source %q is not the object %d:%d that was verified before the move", oldName, wantID.dev, wantID.ino)
	}
	if err := nameRefersTo(oldParent, oldName, srcID); err != nil {
		return fileID{}, errWrap(ErrAmbiguous, "source %q changed before it was moved: %v", oldName, err)
	}
	if err := renameNoReplaceAt(oldParent, oldName, newParent, newName); err != nil {
		// The rename is the last step that can fail without mutating
		// anything; callers treat this as a pre-mutation refusal.
		return fileID{}, err
	}
	// Everything from here on is post-mutation: unprovable states are
	// ErrAmbiguous with every candidate preserved.
	if err := syncRoot(oldParent); err != nil {
		return fileID{}, errWrap(ErrAmbiguous, "source parent could not be synced after the move: %v", err)
	}
	if err := syncRoot(newParent); err != nil {
		return fileID{}, errWrap(ErrAmbiguous, "destination parent could not be synced after the move: %v", err)
	}
	// Test seam: a byte-identical foreign destination swapped in here must
	// fail the destination proof and never be accepted or rolled back.
	layout.runHook(HookAfterMoveRename)
	dst, dstID, dstErr := openPinnedChild(newParent, newName)
	var digest string
	if dstErr == nil {
		// The destination handle is closed exactly once on every return
		// path. The rollback decision and the rollback itself re-prove the
		// destination with fresh opens and never reuse this handle, so the
		// deferred close cannot release a handle the rollback still needs.
		defer dst.Close()
		digest, dstErr = source.TreeDigestRoot(ctx, dst)
	}
	if dstErr == nil && dstID == wantID && digest == wantDigest {
		// The destination name must still identify the proven object after
		// the digest was computed from the same handle.
		if err := nameRefersTo(newParent, newName, dstID); err != nil {
			return fileID{}, errWrap(ErrAmbiguous, "destination %q changed after it was verified: %v", newName, err)
		}
		if err := layout.verify(); err != nil {
			return fileID{}, err
		}
		return dstID, nil
	}
	// Destination proof failed: the rollback authority must be re-proven
	// from a fresh open of the destination — never from the first sample.
	layout.runHook(HookBeforeMoveRollback)
	ro, roID, roErr := openPinnedChild(newParent, newName)
	if roErr != nil {
		return fileID{}, errWrap(ErrAmbiguous, "moved tree %q cannot be re-opened for the rollback decision: %v", newName, roErr)
	}
	defer ro.Close()
	roDigest, roErr := source.TreeDigestRoot(ctx, ro)
	if roErr != nil {
		return fileID{}, errWrap(ErrAmbiguous, "moved tree %q cannot be re-hashed for the rollback decision: %v", newName, roErr)
	}
	if roID != wantID || roDigest != wantDigest {
		return fileID{}, errWrap(ErrAmbiguous, "destination %q is not the object %d:%d that was moved; it is preserved", newName, wantID.dev, wantID.ino)
	}
	if err := nameRefersTo(newParent, newName, roID); err != nil {
		return fileID{}, errWrap(ErrAmbiguous, "destination %q changed while the rollback was being decided; all candidates are preserved: %v", newName, err)
	}
	// The source slot must be empty: content that appeared there is foreign
	// and must never be overwritten by a rollback.
	if occupied, err := childExists(oldParent, oldName); err != nil {
		return fileID{}, err
	} else if occupied {
		return fileID{}, errWrap(ErrAmbiguous, "source %q is occupied; the moved tree %q is preserved at the destination", oldName, newName)
	}
	if err := layout.verify(); err != nil {
		return fileID{}, err
	}
	if err := rollbackMove(ctx, layout, ro, newParent, newName, oldParent, oldName, roID, wantDigest); err != nil {
		return fileID{}, err
	}
	return fileID{}, errWrap(ErrAmbiguous, "moved tree %q cannot be proven as the source content", newName)
}

// rollbackMove returns the re-proven destination tree at fromParent/fromName
// to toParent/toName through the same safe move used for every Store
// transition. The destination root was re-opened by the caller and carries
// the retained identity; only the logical name still naming that object is
// re-proven before the no-replace rename, both parents are synced, and the
// restored tree is re-proven from its destination handle (identity,
// canonical digest, and logical name) before the layout is revalidated. It
// never attempts a further rollback; any failure preserves every candidate
// and reports ErrAmbiguous.
func rollbackMove(ctx context.Context, layout *storeLayout, src *os.Root, fromParent *os.Root, fromName string, toParent *os.Root, toName string, wantID fileID, wantDigest string) error {
	if err := layout.verify(); err != nil {
		return errWrap(ErrAmbiguous, "layout changed while a move was being rolled back: %v", err)
	}
	// src is the caller's re-opened destination; the logical name must
	// still name it before the reverse rename.
	if err := nameRefersTo(fromParent, fromName, wantID); err != nil {
		return errWrap(ErrAmbiguous, "destination %q changed before it could be rolled back: %v", fromName, err)
	}
	if err := renameNoReplaceAt(fromParent, fromName, toParent, toName); err != nil {
		if errors.Is(err, errDestinationExists) {
			return errWrap(ErrAmbiguous, "source %q is occupied by foreign content; the moved tree %q is preserved", toName, fromName)
		}
		return errWrap(ErrAmbiguous, "moved tree %q could not be returned to %q: %v", fromName, toName, err)
	}
	if err := syncRoot(fromParent); err != nil {
		return errWrap(ErrAmbiguous, "moved tree %q was renamed back but %q could not be synced: %v", fromName, toName, err)
	}
	if err := syncRoot(toParent); err != nil {
		return errWrap(ErrAmbiguous, "moved tree %q was renamed back but %q could not be synced: %v", toName, fromName, err)
	}
	dst, dstID, err := openPinnedChild(toParent, toName)
	if err != nil {
		return errWrap(ErrAmbiguous, "restored tree %q cannot be re-opened: %v", toName, err)
	}
	defer dst.Close()
	digest, err := source.TreeDigestRoot(ctx, dst)
	if err != nil {
		return errWrap(ErrAmbiguous, "restored tree %q cannot be hashed: %v", toName, err)
	}
	if dstID != wantID || digest != wantDigest {
		return errWrap(ErrAmbiguous, "restored tree %q cannot be proven as the moved object; all candidates are preserved", toName)
	}
	if err := nameRefersTo(toParent, toName, dstID); err != nil {
		return errWrap(ErrAmbiguous, "restored tree %q changed after it was verified: %v", toName, err)
	}
	if err := layout.verify(); err != nil {
		return errWrap(ErrAmbiguous, "layout changed while a move was being rolled back: %v", err)
	}
	return nil
}

// syncRoot durably syncs the pinned directory handle so a rename between
// two parents is recorded before the caller proceeds.
func syncRoot(root *os.Root) error {
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// moveNonTree moves the single-component non-directory node oldName below
// oldParent onto newName below newParent with a no-replace rename, proven
// by physical identity only (the node is deliberately not a digestable
// Skill tree); a failure after the rename preserves it with ErrAmbiguous.
func moveNonTree(layout *storeLayout, oldParent *os.Root, oldName string, newParent *os.Root, newName string, wantID fileID) error {
	if err := layout.verify(); err != nil {
		return err
	}
	if err := proveNonTree(oldParent, oldName, wantID); err != nil {
		return err
	}
	if err := renameNoReplaceAt(oldParent, oldName, newParent, newName); err != nil {
		// The last step that can fail without mutating anything.
		return err
	}
	if err := syncRoot(oldParent); err != nil {
		return errWrap(ErrAmbiguous, "source parent could not be synced after the move: %v", err)
	}
	if err := syncRoot(newParent); err != nil {
		return errWrap(ErrAmbiguous, "destination parent could not be synced after the move: %v", err)
	}
	if err := proveNonTree(newParent, newName, wantID); err != nil {
		return err
	}
	return layout.verify()
}

// proveNonTree requires name below parent to still identify the given
// object and that object to still not be a directory.
func proveNonTree(parent *os.Root, name string, wantID fileID) error {
	info, err := parent.Lstat(name)
	if err != nil {
		return errWrap(ErrAmbiguous, "%q cannot be re-opened for the move proof: %v", name, err)
	}
	id, err := fileIDOf(info)
	if err != nil {
		return errWrap(ErrAmbiguous, "%q cannot be identified: %v", name, err)
	}
	if id != wantID {
		return errWrap(ErrAmbiguous, "%q is not the object %d:%d that was verified before the move", name, wantID.dev, wantID.ino)
	}
	if info.IsDir() {
		return errWrap(ErrAmbiguous, "%q is a directory; a non-tree move never displaces directories", name)
	}
	return nil
}
