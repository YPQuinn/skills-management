package source

import (
	"fmt"
	"os"
	"strings"
)

// openPinnedDir opens the single-component directory name below the pinned
// parent and proves from the opened handle that it is the very object the
// parent entry named: a symlink or non-directory is refused before opening,
// and the opened root must be the same file as the pre-open Lstat. The
// returned identity comes from the already-open handle, never from a
// mutable pathname.
func openPinnedDir(parent *os.Root, name string) (*os.Root, fileID, error) {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
		return nil, fileID{}, fmt.Errorf("invalid single-component name %q", name)
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, fileID{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fileID{}, fmt.Errorf("%s is a symlink; refusing to follow it", name)
	}
	if !info.IsDir() {
		return nil, fileID{}, fmt.Errorf("%s is not a directory", name)
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, fileID{}, err
	}
	st, err := child.Stat(".")
	if err != nil {
		child.Close()
		return nil, fileID{}, err
	}
	if !os.SameFile(info, st) {
		child.Close()
		return nil, fileID{}, fmt.Errorf("%s changed while it was being opened", name)
	}
	return child, fileIDOf(st), nil
}

// nameStillRefers reports whether the single-component name below the
// pinned parent still identifies the object with the given physical
// identity, so a pathname swap is detected after an object was opened.
func nameStillRefers(parent *os.Root, name string, want fileID) error {
	info, err := parent.Lstat(name)
	if err != nil {
		return err
	}
	if fileIDOf(info) != want {
		return fmt.Errorf("%s no longer refers to the expected object", name)
	}
	return nil
}
