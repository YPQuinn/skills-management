package skillstore

import (
	"context"
	"errors"
	"os"
)

// testDrainChildHook, when set, runs after drainChild sampled a child's
// identity and before that child is opened for the recursive drain (a
// directory) or for the pinned removal (a regular file). Tests swap the
// child for a foreign node here, exercising the real ReadDir→Lstat→open
// window that direct drainTree calls cannot reach. Nil in production;
// tests must reset it.
var testDrainChildHook func(parent *os.Root, name string)

// drainVerifiedTree is the destructive-drain boundary check: the tree at
// parent/name is re-verified immediately before its recursive drain with
// all three bindings — exact physical root identity, the logical name
// still identifying that root, and the current canonical digest equal to
// the operation/proof/witness expected digest — sampled from one pinned
// open. An in-place mutation inside the same directory after an earlier
// verification (the deterministic hook window or a concurrent writer) is
// refused with ErrAmbiguous and preserved; only then is the tree drained
// by identity. The drain itself re-proves the identity once more.
func drainVerifiedTree(ctx context.Context, parent *os.Root, name string, wantID fileID, wantDigest string) error {
	root, id, digest, err := openTreeDigest(ctx, parent, name)
	if err != nil {
		return errWrap(ErrAmbiguous, "%s cannot be re-verified before its drain: %v", name, err)
	}
	defer root.Close()
	if id != wantID || digest != wantDigest {
		return errWrap(ErrAmbiguous, "%s changed after it was verified; it is preserved", name)
	}
	if err := nameRefersTo(parent, name, id); err != nil {
		return errWrap(ErrAmbiguous, "%s changed before its drain: %v", name, err)
	}
	return drainTree(parent, name, id)
}

// drainTree removes the pinned single-component tree at parent/name: the
// tree is opened and identity-pinned, the parent entry is re-proven,
// every child is drained recursively through pinned child handles, and the
// final directory entry is removed through deleteVerified — an atomic
// detach into a unique trash slot followed by the verified unlink of the
// captured object — never RemoveAll of a mutable name. When expected is
// non-zero the current object must carry exactly that identity, so a
// swapped tree is preserved instead of drained, and a name that vanished
// after the caller proved the object is reported as ErrAmbiguous instead
// of a silent success. A swapped non-empty foreign tree makes the final
// trash unlink fail (ENOTEMPTY) and is preserved.
func drainTree(parent *os.Root, name string, expected fileID) error {
	var seq uint64
	return drainTreeSeq(parent, name, expected, &seq)
}

// drainTreeSeq is the recursive drain behind drainTree; seq is the shared
// invocation-unique trash-slot counter of one drain.
func drainTreeSeq(parent *os.Root, name string, expected fileID, seq *uint64) error {
	root, id, err := openPinnedChild(parent, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if expected != (fileID{}) {
				// The caller proved a specific object; a name that no
				// longer exists cannot certify that object's cleanup.
				return errWrap(ErrAmbiguous, "%s vanished before it could be drained", name)
			}
			// Already absent: removal is idempotent like the removed
			// RemoveAll callers.
			return nil
		}
		return err
	}
	defer root.Close()
	if expected != (fileID{}) && id != expected {
		return errWrap(ErrAmbiguous, "%s no longer refers to the object that was verified", name)
	}
	if err := nameRefersTo(parent, name, id); err != nil {
		return errWrap(ErrAmbiguous, "%s changed while it was being drained: %v", name, err)
	}
	if err := drainChildren(root, seq); err != nil {
		return err
	}
	// The tree is empty: the final directory entry is detached into a
	// unique trash slot and only the re-verified captured object is
	// unlinked.
	return deleteVerified(parent, name, id, seq)
}

// drainChildren removes every child of the pinned root: directories recurse
// through their pinned child roots carrying the identity sampled from the
// child's Lstat, so a foreign directory swapped in after the listing can
// never be drained; non-directories are identity-pinned before their
// removal so a swapped node is never removed.
func drainChildren(root *os.Root, seq *uint64) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := drainChild(root, e, seq); err != nil {
			return err
		}
	}
	return nil
}

// drainChild removes one listed child: a directory recurses with the
// identity sampled from its own Lstat (so a foreign directory swapped in
// after the listing is refused and preserved), a regular file is pinned
// through its open handle before removal, and every check that could be
// redirected by a swap is reported as ErrAmbiguous so a cleanup can never
// be certified from an unproven name. The removal itself runs through
// deleteVerified, so the logical name is never unlinked directly.
func drainChild(root *os.Root, e os.DirEntry, seq *uint64) error {
	name := e.Name()
	info, err := root.Lstat(name)
	if err != nil {
		return errWrap(ErrAmbiguous, "%s vanished before it could be drained", name)
	}
	id, err := fileIDOf(info)
	if err != nil {
		return errWrap(ErrAmbiguous, "%s cannot be identified: %v", name, err)
	}
	if info.IsDir() {
		// Test seam: a foreign directory swapped in here must be refused by
		// the recursive drain's identity check and preserved.
		if testDrainChildHook != nil {
			testDrainChildHook(root, name)
		}
		// Recurse with the identity sampled from the child's own Lstat: the
		// recursive drain must drain the very directory that was listed,
		// never a foreign directory swapped into the slot in between.
		return drainTreeSeq(root, name, id, seq)
	}
	if info.Mode().IsRegular() {
		// Test seam: a foreign regular file swapped in here must be refused
		// by the open-handle identity check below and preserved.
		if testDrainChildHook != nil {
			testDrainChildHook(root, name)
		}
		// Pin the file itself so the removal cannot hit a swapped node.
		f, err := openDrainFile(root, name)
		if err != nil {
			return errWrap(ErrAmbiguous, "%s could not be opened for removal: %v", name, err)
		}
		fid, err := fileIDFromOpen(f)
		f.Close()
		if err != nil {
			return errWrap(ErrAmbiguous, "%s cannot be identified from its open handle: %v", name, err)
		}
		if fid != id {
			return errWrap(ErrAmbiguous, "%s changed while it was being opened for removal", name)
		}
	}
	return deleteVerified(root, name, id, seq)
}
