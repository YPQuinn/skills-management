package skillstore

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// removeWitnessIfPresent removes the witness at parent/name when it is
// still present, through the deterministic removal; an absent witness is
// already removed. A foreign or malformed object at the name is preserved
// with ErrAmbiguous.
func removeWitnessIfPresent(parent *os.Root, name string, opID int64, action witnessAction) error {
	w, err := readTransientWitness(parent, name, opID, action)
	if err != nil {
		return errWrap(ErrAmbiguous, "witness %q cannot be removed: %v", name, err)
	}
	if w == nil {
		return nil
	}
	return removeWitnessFile(parent, name, w)
}

// removeWitnessFile removes the witness at parent/name through a
// deterministic no-replace detach into its tombstone, an identity
// revalidation of the captured object, and the unlink of only the
// re-verified captured object — never a direct removal of the canonical
// name. A foreign object captured by the detach fails the revalidation and
// is preserved in the tombstone; a canonical name that no longer refers to
// the witness (with no tombstone) is foreign and preserved.
func removeWitnessFile(parent *os.Root, name string, w *transientWitness) error {
	tomb := witnessTombName(name)
	canonicalErr := nameRefersTo(parent, name, w.selfID)
	tombExists, err := childExists(parent, tomb)
	if err != nil {
		return err
	}
	switch {
	case canonicalErr == nil && tombExists:
		return errWrap(ErrAmbiguous, "witness %q and its tombstone coexist; it is preserved", name)
	case canonicalErr != nil && tombExists:
		// The detach already happened: resume the unlink of the captured
		// object.
		return resumeWitnessRemoval(parent, name, w.opID, w.action, w.selfID)
	case canonicalErr != nil:
		return errWrap(ErrAmbiguous, "witness %q is not the object %d:%d this operation recorded; it is preserved",
			name, w.selfID.dev, w.selfID.ino)
	}
	if err := renameNoReplaceAt(parent, name, parent, tomb); err != nil {
		if errors.Is(err, errDestinationExists) {
			return errWrap(ErrAmbiguous, "witness tombstone %q already exists; it is preserved", tomb)
		}
		return errWrap(ErrAmbiguous, "witness %q could not be detached for its removal: %v", name, err)
	}
	if err := syncRoot(parent); err != nil {
		return errWrap(ErrAmbiguous, "witness %q was detached but its parent could not be synced: %v", name, err)
	}
	return resumeWitnessRemoval(parent, name, w.opID, w.action, w.selfID)
}

// resumeWitnessRemoval unlinks the witness tombstone only after proving it
// is the very witness file this operation wrote: plain single-link
// regular-file policy, strict canonical content, the recorded operation
// and action match, the recorded self identity equals the actual object,
// and — when the caller already holds the canonical witness — equals that
// witness's identity. The tombstone name must still identify the object
// before the unlink. An absent tombstone is already removed. Anything else
// is preserved with ErrAmbiguous.
func resumeWitnessRemoval(parent *os.Root, name string, opID int64, action witnessAction, selfID fileID) error {
	tomb := witnessTombName(name)
	info, err := parent.Lstat(tomb)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errWrap(ErrAmbiguous, "witness tombstone %q cannot be identified: %v", tomb, err)
	}
	if err := witnessFilePolicy(info); err != nil {
		return errWrap(ErrAmbiguous, "witness tombstone %q %v; it is preserved", tomb, err)
	}
	f, err := parent.OpenFile(tomb, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return errWrap(ErrAmbiguous, "witness tombstone %q cannot be opened: %v", tomb, err)
	}
	fid, err := fileIDFromOpen(f)
	if err != nil {
		f.Close()
		return errWrap(ErrAmbiguous, "witness tombstone %q cannot be identified: %v", tomb, err)
	}
	if !sameIdentity(info, fid) {
		f.Close()
		return errWrap(ErrAmbiguous, "witness tombstone %q changed while it was being opened; it is preserved", tomb)
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return errWrap(ErrAmbiguous, "witness tombstone %q cannot be read: %v", tomb, err)
	}
	if err := nameRefersTo(parent, tomb, fid); err != nil {
		return errWrap(ErrAmbiguous, "witness tombstone %q changed while it was being read; it is preserved: %v", tomb, err)
	}
	w, err := parseTransientWitness(data)
	if err != nil {
		return errWrap(ErrAmbiguous, "witness tombstone %q is malformed; it is preserved: %v", tomb, err)
	}
	if w.opID != opID || w.action != action {
		return errWrap(ErrAmbiguous, "witness tombstone %q belongs to a different operation or slot; it is preserved", tomb)
	}
	if w.selfID != fid || (selfID != (fileID{}) && w.selfID != selfID) {
		return errWrap(ErrAmbiguous, "witness tombstone %q is not the object this operation recorded; it is preserved", tomb)
	}
	if err := nameRefersTo(parent, tomb, fid); err != nil {
		return errWrap(ErrAmbiguous, "witness tombstone %q changed before its verified removal; it is preserved: %v", tomb, err)
	}
	return parent.Remove(tomb)
}
