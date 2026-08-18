//go:build linux

package distribution

import "golang.org/x/sys/unix"

func renameNoReplaceFD(oldFD int, oldName string, newFD int, newName string) error {
	if err := unix.Renameat2(oldFD, oldName, newFD, newName, unix.RENAME_NOREPLACE); err != nil {
		if err == unix.EEXIST {
			return errDestinationExists
		}
		return err
	}
	return nil
}
