package distribution

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"skillctl/internal/source"
)

var ErrEntryExists = errors.New("an entry already exists at the link path")
var ErrSlotOccupied = errors.New("the isolation directory is occupied")

// Same-UID race (syscall evidence): symlinkat(2) is atomic and fails with
// EEXIST, so a successful create cannot overwrite. renameat2/renameatx_np
// with RENAME_NOREPLACE/RENAME_EXCL is atomic, so isolation cannot
// overwrite a concurrently created slug. unlinkat(2) names a directory
// entry, not an inode. Between readlinkat(isolation_fd, "link") and
// unlinkat(isolation_fd, "link"), a same-UID writer who listed the
// unguessable 0700 directory can replace "link". Unix has no portable
// unlink-by-inode: Linux unlinkat(fd, "", AT_EMPTY_PATH) is unspecified
// for O_PATH symlinks and is unavailable on Darwin; 0700 does not exclude
// the creating UID. Pinned O_NOFOLLOW directory fds plus a 128-bit
// isolation name is the strongest decision-06 scheme: we never unlink an
// entry we have not just verified through that fd, and we never authorize
// deletion by raw target or a guessable name.

func EnsureContainer(containerPath string) error {
	container, err := openContainerHandle(containerPath, true)
	if err != nil {
		return err
	}
	return container.Close()
}

// CreateLink writes the Managed Link with symlinkat on the pinned Target
// directory fd. The call is atomic and fails with ErrEntryExists instead
// of overwriting. There is no visible temporary name.
func CreateLink(containerPath, slug, rawTarget string) error {
	container, err := openContainerHandle(containerPath, true)
	if err != nil {
		return err
	}
	defer container.Close()
	if err := container.symlink(rawTarget, slug); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return ErrEntryExists
		}
		return fmt.Errorf("creating the link: %v", err)
	}
	return nil
}

type RemoveResult int

const (
	RemoveDone RemoveResult = iota
	RemoveAbsent
	RemoveMismatch
)

// CreateIsolationDir mkdirat(0700)+openat(O_NOFOLLOW) the intent's private
// isolation directory under the pinned Target fd.
func CreateIsolationDir(containerPath, isolation string) error {
	parent, err := openContainerHandle(containerPath, false)
	if err != nil {
		return err
	}
	defer parent.Close()
	iso, err := parent.mkdirIsolation(isolation)
	if err != nil {
		return err
	}
	return iso.Close()
}

// IsolateInto no-replace-renames the slug into IsolatedEntry inside the
// already-created isolation directory, using the pinned directory fds.
func IsolateInto(containerPath, slug, isolation string) error {
	parent, iso, err := openIsolation(containerPath, isolation)
	if err != nil {
		return err
	}
	defer parent.Close()
	defer iso.Close()
	if err := parent.renameTo(slug, iso, IsolatedEntry); err != nil {
		if errors.Is(err, errDestinationExists) {
			return ErrSlotOccupied
		}
		return err
	}
	return nil
}

// ProbeIsolationEntry stats IsolatedEntry through the pinned isolation
// directory fd. A missing isolation dir or missing IsolatedEntry is
// os.ErrNotExist so recovery can tell "dir created, not yet isolated"
// from "already isolated".
func ProbeIsolationEntry(containerPath, isolation string) (kind, raw string, err error) {
	parent, iso, err := openIsolation(containerPath, isolation)
	if err != nil {
		return "", "", err
	}
	defer parent.Close()
	defer iso.Close()
	return probeEntry(iso, IsolatedEntry)
}

func openIsolation(containerPath, isolation string) (parent, iso *containerHandle, err error) {
	parent, err = openContainerHandle(containerPath, false)
	if err != nil {
		return nil, nil, err
	}
	iso, err = parent.openDirNoFollow(isolation)
	if err != nil {
		parent.Close()
		return nil, nil, err
	}
	return parent, iso, nil
}

func removeEmptyIsolation(parent *containerHandle, isolation string) {
	_ = parent.rmdir(isolation)
}

// DiscardIsolation rmdirs an empty private isolation directory. A
// non-empty directory is left untouched.
func DiscardIsolation(containerPath, isolation string) error {
	parent, err := openContainerHandle(containerPath, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := parent.rmdir(isolation); errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return err
	}
}

// FinishIsolatedRemove verifies IsolatedEntry through the pinned isolation
// fd, then unlinks only that name. A mismatch restores onto the slug
// without overwrite. Unproven isolation contents are never deleted.
func FinishIsolatedRemove(containerPath, slug, rawTarget, isolation string) (RemoveResult, error) {
	parent, iso, err := openIsolation(containerPath, isolation)
	if errors.Is(err, os.ErrNotExist) {
		return RemoveAbsent, nil
	}
	if err != nil {
		return RemoveAbsent, err
	}
	defer parent.Close()
	defer iso.Close()
	kind, raw, err := probeEntry(iso, IsolatedEntry)
	if errors.Is(err, os.ErrNotExist) {
		removeEmptyIsolation(parent, isolation)
		return RemoveAbsent, nil
	}
	if err != nil {
		return RemoveAbsent, fmt.Errorf("verifying isolated %s: %v", slug, err)
	}
	if kind != KindSymlink || raw != rawTarget {
		if err := iso.renameTo(IsolatedEntry, parent, slug); err != nil {
			return RemoveAbsent, fmt.Errorf("restoring changed entry %s without overwrite: %v", slug, err)
		}
		removeEmptyIsolation(parent, isolation)
		return RemoveMismatch, nil
	}
	if err := iso.unlink(IsolatedEntry); err != nil {
		return RemoveAbsent, fmt.Errorf("removing %s: %v", slug, err)
	}
	removeEmptyIsolation(parent, isolation)
	return RemoveDone, nil
}

func RemoveManagedLink(containerPath, slug, rawTarget, isolation string) (RemoveResult, error) {
	if err := CreateIsolationDir(containerPath, isolation); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RemoveAbsent, nil
		}
		return RemoveAbsent, err
	}
	if err := IsolateInto(containerPath, slug, isolation); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = DiscardIsolation(containerPath, isolation)
			return RemoveAbsent, nil
		}
		return RemoveAbsent, err
	}
	return FinishIsolatedRemove(containerPath, slug, rawTarget, isolation)
}

func ProbeLink(containerPath, slug string) (kind, raw string, err error) {
	container, err := openContainerHandle(containerPath, false)
	if err != nil {
		return "", "", err
	}
	defer container.Close()
	return probeEntry(container, slug)
}

func ProbeAdoption(containerPath, slug, expectedPath string) (string, error) {
	container, err := openContainerHandle(containerPath, false)
	if err != nil {
		return "", fmt.Errorf("opening the Target container: %v", err)
	}
	defer container.Close()
	kind, raw, err := probeEntry(container, slug)
	if err != nil {
		return "", fmt.Errorf("inspecting the link: %v", err)
	}
	if kind != KindSymlink {
		return "", fmt.Errorf("the entry is not a symlink")
	}
	matches, err := container.targetMatches(raw, expectedPath)
	if err != nil || !matches {
		return "", fmt.Errorf("the link does not point at the Store Skill %s", expectedPath)
	}
	if err := source.ValidateSkillDir(expectedPath); err != nil {
		return "", fmt.Errorf("the link destination is not a Skill directory")
	}
	return raw, nil
}

func probeEntry(container *containerHandle, name string) (kind, raw string, err error) {
	st, err := container.stat(name)
	if err != nil {
		return "", "", err
	}
	kind = statKind(st)
	if kind == KindSymlink {
		raw, err = container.readlink(name)
	}
	return kind, raw, err
}
