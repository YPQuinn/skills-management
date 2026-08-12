//go:build darwin

package skillstore

import (
	"os"

	"golang.org/x/sys/unix"
)

// renameNoReplaceAt atomically renames the single-component name oldName
// below the pinned oldParent onto newName below the pinned newParent,
// failing with errDestinationExists (wrapping EEXIST) when newName already
// exists. Both parents are pinned os.Root handles and both names are single
// path components (slugs and numeric operation/Skill ids), so a post-layout
// path swap can never redirect the rename outside the Store and a fresh
// install can never overwrite content that appeared after the last
// existence check.
func renameNoReplaceAt(oldParent *os.Root, oldName string, newParent *os.Root, newName string) error {
	if err := validateRenameName(oldName); err != nil {
		return err
	}
	if err := validateRenameName(newName); err != nil {
		return err
	}
	old, err := oldParent.Open(".")
	if err != nil {
		return err
	}
	defer old.Close()
	neu, err := newParent.Open(".")
	if err != nil {
		return err
	}
	defer neu.Close()
	if err := unix.RenameatxNp(int(old.Fd()), oldName, int(neu.Fd()), newName, unix.RENAME_EXCL); err != nil {
		if err == unix.EEXIST {
			return errDestinationExists
		}
		return err
	}
	return nil
}
