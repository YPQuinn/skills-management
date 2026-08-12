package skillstore

import (
	"errors"
	"fmt"
	"os"
)

// testBeforeDeleteUnlinkHook, when set, runs after the object was detached
// into its trash slot and re-verified there, immediately before the trash
// slot is unlinked. The original logical name is already empty at this
// point. Tests swap a foreign node into the original name (or the trash
// name) here and assert that only the verified owned object can ever be
// unlinked. Nil in production; tests must reset it.
var testBeforeDeleteUnlinkHook func(parent *os.Root, name, trashName string)

// deleteVerified is the Store's only deletion primitive: it removes the
// single-component entry name below the pinned parent, and only if that
// entry is the object with the physical identity wantID. Every direct
// Remove call site (operation-directory cleanup, proof removal, tree
// drains) funnels through it, so no path performs a check-then-Remove on a
// logical name that a same-UID writer could swap between the identity
// check and the unlink.
//
// The removal is a detach-then-delete:
//
//  1. detach: the name is atomically renamed (no-replace) into a unique
//     trash slot below the same pinned parent. Whatever the name referred
//     to at the instant of the rename is what the slot captures, and the
//     original logical name becomes empty, so a foreign object that
//     appears there afterwards is never unlinked.
//  2. delete: the captured object is re-proven once from the trash slot —
//     its physical identity must still equal wantID — the trash slot's
//     name must still identify that object, and only then is the slot
//     unlinked. A foreign object captured by the detach fails the
//     re-proof and is preserved in the trash slot with ErrAmbiguous.
//
// The trash slot is invocation-unique (a monotonically increasing suffix)
// and slot-exclusive (the no-replace rename fails when the slot already
// exists, so an existing entry is never overwritten), and the parent is
// synced after the detach. Guarantee boundary: a non-cooperative same-UID
// writer can still race the final unlink syscall of the trash slot itself
// — POSIX offers no atomic unlink-of-identity — but the Store never
// unlinks by the original logical name, so a foreign object swapped in
// after the detach is preserved at that name, and a swap of the trash slot
// is detected by the pre-unlink re-proof and reported as ErrAmbiguous with
// the object preserved.
func deleteVerified(parent *os.Root, name string, wantID fileID, seq *uint64) error {
	if err := nameRefersTo(parent, name, wantID); err != nil {
		return errWrap(ErrAmbiguous, "%s no longer refers to the verified object: %v", name, err)
	}
	*seq++
	trashName := fmt.Sprintf(".skill-del-%d", *seq)
	if err := renameNoReplaceAt(parent, name, parent, trashName); err != nil {
		if errors.Is(err, errDestinationExists) {
			return errWrap(ErrAmbiguous, "trash slot %q for %q already exists; nothing was removed", trashName, name)
		}
		return errWrap(ErrAmbiguous, "%s could not be detached for its verified removal: %v", name, err)
	}
	if err := syncRoot(parent); err != nil {
		return errWrap(ErrAmbiguous, "%s was detached to %q but its parent could not be synced: %v", name, trashName, err)
	}
	// The captured object must still be the verified object: a foreign
	// object renamed into the slot before the detach is refused and
	// preserved.
	if err := verifyTrashObject(parent, trashName, wantID); err != nil {
		return errWrap(ErrAmbiguous, "detached object %q cannot be proven as the verified object %d:%d; it is preserved in %q: %v",
			name, wantID.dev, wantID.ino, trashName, err)
	}
	// Test seam: the original logical name is empty here; a foreign node
	// swapped into it is never unlinked.
	if testBeforeDeleteUnlinkHook != nil {
		testBeforeDeleteUnlinkHook(parent, name, trashName)
	}
	if err := nameRefersTo(parent, trashName, wantID); err != nil {
		return errWrap(ErrAmbiguous, "trash slot %q changed before the verified removal; it is preserved", trashName)
	}
	if err := parent.Remove(trashName); err != nil {
		return errWrap(ErrAmbiguous, "trash slot %q could not be removed: %v", trashName, err)
	}
	return nil
}

// verifyTrashObject opens the detached object in the trash slot exactly
// once and requires its physical identity to equal wantID: directories are
// pinned through their root handle, regular files through a no-follow open
// of the entry, and anything else is refused.
func verifyTrashObject(parent *os.Root, trashName string, wantID fileID) error {
	info, err := parent.Lstat(trashName)
	if err != nil {
		return err
	}
	if info.IsDir() {
		child, id, err := openPinnedChild(parent, trashName)
		if err != nil {
			return err
		}
		child.Close()
		if id != wantID {
			return fmt.Errorf("identity %d:%d does not match the verified identity %d:%d", id.dev, id.ino, wantID.dev, wantID.ino)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", trashName)
	}
	f, err := openDrainFile(parent, trashName)
	if err != nil {
		return err
	}
	fid, err := fileIDFromOpen(f)
	f.Close()
	if err != nil {
		return err
	}
	if fid != wantID {
		return fmt.Errorf("identity %d:%d does not match the verified identity %d:%d", fid.dev, fid.ino, wantID.dev, wantID.ino)
	}
	return nil
}
