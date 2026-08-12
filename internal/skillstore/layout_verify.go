package skillstore

import (
	"os"
	"strconv"
)

// verify re-proves that every logical Store path still names the pinned
// handle it was opened as: the configured Store path must still identify
// l.store, Store/.skillctl must still identify l.internal, and each
// internal child name must still identify its pinned root. A detached or
// swapped logical layout is reported as ErrAmbiguous so no public Store
// mutation can be blessed as successful through a handle the configured
// Store no longer names.
func (l *storeLayout) verify() error {
	info, err := os.Lstat(l.rootPath)
	if err != nil {
		return errWrap(ErrAmbiguous, "Store root %q is no longer present: %v", l.rootPath, err)
	}
	if fid, ferr := fileIDOf(info); ferr != nil || fid != l.storeID {
		return errWrap(ErrAmbiguous, "Store root %q no longer identifies the opened root", l.rootPath)
	}
	if err := l.verifyChild(l.store, ".skillctl", l.internalID); err != nil {
		return err
	}
	for _, c := range []struct {
		name string
		id   fileID
	}{
		{"staging", l.stagingID},
		{"baselines", l.baselinesID},
		{"previous", l.previousID},
		{"recovery", l.recoveryID},
	} {
		if err := l.verifyChild(l.internal, c.name, c.id); err != nil {
			return err
		}
	}
	return nil
}

// verifyChild reports that the single-component name below parent still
// identifies the pinned object with the given identity, wrapping any
// mismatch in ErrAmbiguous.
func (l *storeLayout) verifyChild(parent *os.Root, name string, want fileID) error {
	if err := nameRefersTo(parent, name, want); err != nil {
		return errWrap(ErrAmbiguous, "internal layout %q no longer identifies its pinned object: %v", name, err)
	}
	return nil
}

// verifyOpDir re-proves that staging/<opID> still names the pinned
// operation directory, so an operation never mutates or reports success
// through a detached operation directory. A directory that cannot be
// identified, or whose logical entry no longer names it, is wrapped in
// ErrAmbiguous.
func (l *storeLayout) verifyOpDir(opDir *os.Root, opID int64) error {
	id, err := rootID(opDir)
	if err != nil {
		return errWrap(ErrAmbiguous, "operation directory %d cannot be identified: %v", opID, err)
	}
	return l.verifyChild(l.staging, strconv.FormatInt(opID, 10), id)
}
