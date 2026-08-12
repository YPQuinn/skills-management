//go:build darwin || linux

package skillstore

import (
	"os"

	"golang.org/x/sys/unix"
)

// openDrainFile opens the single-component regular file below the pinned
// parent without following a final symlink, so the removal of a drained
// tree can prove the file it removes is the file the parent entry named.
func openDrainFile(parent *os.Root, name string) (*os.File, error) {
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
