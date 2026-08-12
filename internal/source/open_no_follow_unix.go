//go:build darwin || linux

package source

import (
	"os"

	"golang.org/x/sys/unix"
)

// openNoFollowFile opens the single-component name below the pinned parent
// without following a final symlink, so a regular file swapped for an
// internal symlink between classification and open can never redirect the
// hash to another object.
func openNoFollowFile(parent *os.Root, name string) (*os.File, error) {
	dir, err := parent.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
