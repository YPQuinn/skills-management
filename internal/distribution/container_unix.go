//go:build darwin || linux

package distribution

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type containerHandle struct {
	fd   int
	path string
}

func openContainerHandle(path string, create bool) (*containerHandle, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("the Target container path must be absolute")
	}
	clean := filepath.Clean(path)
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, name := range strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator)) {
		if name == "" {
			continue
		}
		next, openErr := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil && create && errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, name, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				unix.Close(fd)
				return nil, fmt.Errorf("creating Target directory %s: %w", name, mkdirErr)
			}
			next, openErr = unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		unix.Close(fd)
		if openErr != nil {
			if errors.Is(openErr, unix.ENOENT) {
				return nil, fmt.Errorf("%w: %s", os.ErrNotExist, clean)
			}
			return nil, fmt.Errorf("opening Target directory %s without following links: %w", name, openErr)
		}
		fd = next
	}
	return &containerHandle{fd: fd, path: clean}, nil
}

func (c *containerHandle) Close() error { return unix.Close(c.fd) }

func validEntryName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsRune(name, filepath.Separator)
}

func (c *containerHandle) stat(name string) (unix.Stat_t, error) {
	var st unix.Stat_t
	if !validEntryName(name) {
		return st, fmt.Errorf("invalid Target entry name %q", name)
	}
	if err := unix.Fstatat(c.fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return st, os.ErrNotExist
		}
		return st, err
	}
	return st, nil
}

func statKind(st unix.Stat_t) string {
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFLNK:
		return KindSymlink
	case unix.S_IFDIR:
		return KindDir
	case unix.S_IFREG:
		return KindFile
	default:
		return KindOther
	}
}

func (c *containerHandle) readlink(name string) (string, error) {
	if !validEntryName(name) {
		return "", fmt.Errorf("invalid Target entry name %q", name)
	}
	for size := 256; size <= 64*1024; size *= 2 {
		buf := make([]byte, size)
		n, err := unix.Readlinkat(c.fd, name, buf)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				return "", os.ErrNotExist
			}
			return "", err
		}
		if n < len(buf) {
			return string(buf[:n]), nil
		}
	}
	return "", fmt.Errorf("the symlink target is too long")
}

func (c *containerHandle) symlink(rawTarget, name string) error {
	if !validEntryName(name) {
		return fmt.Errorf("invalid Target entry name %q", name)
	}
	return unix.Symlinkat(rawTarget, c.fd, name)
}

func (c *containerHandle) unlink(name string) error {
	if !validEntryName(name) {
		return fmt.Errorf("invalid Target entry name %q", name)
	}
	if err := unix.Unlinkat(c.fd, name, 0); errors.Is(err, unix.ENOENT) {
		return os.ErrNotExist
	} else {
		return err
	}
}

func (c *containerHandle) renameNoReplace(oldName, newName string) error {
	return c.renameTo(oldName, c, newName)
}

func (c *containerHandle) renameTo(oldName string, dest *containerHandle, destName string) error {
	if !validEntryName(oldName) || !validEntryName(destName) {
		return fmt.Errorf("invalid Target entry name")
	}
	err := renameNoReplaceFD(c.fd, oldName, dest.fd, destName)
	if errors.Is(err, unix.ENOENT) {
		return os.ErrNotExist
	}
	return err
}

func (c *containerHandle) mkdirIsolation(name string) (*containerHandle, error) {
	if !validIsolationName(name) {
		return nil, fmt.Errorf("invalid isolation directory name")
	}
	if err := unix.Mkdirat(c.fd, name, 0o700); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return nil, ErrSlotOccupied
		}
		return nil, fmt.Errorf("creating the isolation directory: %v", err)
	}
	return c.openDirNoFollow(name)
}

func (c *containerHandle) openDirNoFollow(name string) (*containerHandle, error) {
	if !validIsolationName(name) {
		return nil, fmt.Errorf("invalid isolation directory name")
	}
	fd, err := unix.Openat(c.fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("opening the isolation directory without following links: %w", err)
	}
	return &containerHandle{fd: fd, path: filepath.Join(c.path, name)}, nil
}

func (c *containerHandle) rmdir(name string) error {
	if !validIsolationName(name) {
		return fmt.Errorf("invalid isolation directory name")
	}
	if err := unix.Unlinkat(c.fd, name, unix.AT_REMOVEDIR); errors.Is(err, unix.ENOENT) {
		return os.ErrNotExist
	} else {
		return err
	}
}

func (c *containerHandle) targetMatches(rawTarget, expectedPath string) (bool, error) {
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC
	var targetFD int
	var err error
	if filepath.IsAbs(rawTarget) {
		targetFD, err = unix.Open(rawTarget, flags, 0)
	} else {
		targetFD, err = unix.Openat(c.fd, rawTarget, flags, 0)
	}
	if err != nil {
		return false, err
	}
	defer unix.Close(targetFD)
	expectedFD, err := unix.Open(expectedPath, flags|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	defer unix.Close(expectedFD)
	var targetStat, expectedStat unix.Stat_t
	if err := unix.Fstat(targetFD, &targetStat); err != nil {
		return false, err
	}
	if err := unix.Fstat(expectedFD, &expectedStat); err != nil {
		return false, err
	}
	return targetStat.Dev == expectedStat.Dev && targetStat.Ino == expectedStat.Ino, nil
}
