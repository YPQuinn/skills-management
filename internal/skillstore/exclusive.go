package skillstore

import (
	"fmt"
	"os"
)

// testAfterMkdirHook, when set, runs immediately after a directory was
// created exclusively and before it is opened for pinning. Tests use it to
// exercise the Mkdir→OpenRoot window of createPinnedDirExclusive: a foreign
// node planted there is never blessed as created and any later cleanup must
// refuse it. Nil in production; tests must reset it.
var testAfterMkdirHook func(parent *os.Root, name string)

// testAfterOpDirEntrySync, when set, runs immediately after the operation
// directory entry was durably synced in its staging parent, before Stage
// returns. Tests use it to pin the fsync ordering that makes the operation
// directory reachable after a crash before Install's first live mutation.
// Nil in production; tests must reset it.
var testAfterOpDirEntrySync func(staging *os.Root, opDirID fileID)

// createPinnedDirExclusive creates the single-component directory name
// below the pinned parent atomically and exclusively: a pre-existing entry
// is contradictory evidence and is preserved with ErrAmbiguous, never
// opened as the created directory. The created directory is pinned through
// its open handle, its mode is applied through that handle, and the logical
// name must still identify the pinned object before the creation is
// reported. The sampled identity is returned for the caller's creation
// manifest.
func createPinnedDirExclusive(parent *os.Root, name string, mode os.FileMode) (*os.Root, fileID, error) {
	if err := validateRenameName(name); err != nil {
		return nil, fileID{}, err
	}
	err := parent.Mkdir(name, mode)
	if err != nil {
		if os.IsExist(err) {
			return nil, fileID{}, errWrap(ErrAmbiguous, "%s already exists; it is preserved", name)
		}
		return nil, fileID{}, err
	}
	if testAfterMkdirHook != nil {
		testAfterMkdirHook(parent, name)
	}
	child, childID, err := openPinnedChild(parent, name)
	if err != nil {
		return nil, fileID{}, errWrap(ErrAmbiguous, "%s could not be pinned after its creation: %v", name, err)
	}
	// Test seam: a symlink swapped in here must not redirect the mode: the
	// fchmod runs on the pinned child and the re-proof below fails.
	if testBeforeChmodDirHook != nil {
		testBeforeChmodDirHook(parent, name)
	}
	if err := chmodRootDir(child, mode); err != nil {
		child.Close()
		return nil, fileID{}, err
	}
	if err := nameRefersTo(parent, name, childID); err != nil {
		child.Close()
		return nil, fileID{}, errWrap(ErrAmbiguous, "destination %q changed while its mode was set", name)
	}
	return child, childID, nil
}

// ensurePinnedDir is the layout variant of createPinnedDirExclusive: it
// creates the single-component directory name below the pinned parent when
// absent, and opens and identity-verifies an existing entry without ever
// modifying it. It is used only for the reusable layout directories; every
// constructed operation artifact uses the exclusive variant.
func ensurePinnedDir(parent *os.Root, name string, mode os.FileMode) (*os.Root, fileID, error) {
	if err := validateRenameName(name); err != nil {
		return nil, fileID{}, err
	}
	err := parent.Mkdir(name, mode)
	if err == nil {
		child, childID, err := openPinnedChild(parent, name)
		if err != nil {
			return nil, fileID{}, err
		}
		// Test seam: a symlink swapped in here must not redirect the mode.
		if testBeforeChmodDirHook != nil {
			testBeforeChmodDirHook(parent, name)
		}
		if err := chmodRootDir(child, mode); err != nil {
			child.Close()
			return nil, fileID{}, err
		}
		if err := nameRefersTo(parent, name, childID); err != nil {
			child.Close()
			return nil, fileID{}, fmt.Errorf("destination %q changed while its mode was set", name)
		}
		return child, childID, nil
	}
	if os.IsExist(err) {
		child, childID, err := openPinnedChild(parent, name)
		return child, childID, err
	}
	return nil, fileID{}, err
}

// chmodRootDir applies mode to the pinned directory root through its own
// handle: the root's directory FD is opened and fchmod'd, so no mutable
// pathname is ever re-traversed and a symlink swap can never redirect the
// mode change to a foreign target.
func chmodRootDir(child *os.Root, mode os.FileMode) error {
	f, err := child.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Chmod(mode)
}
