package skillstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"skillctl/internal/source"
)

// fileID is the physical identity of one directory: its filesystem device
// and inode. Renames preserve the identity, so it proves that a directory
// moved by the operation is the exact directory that was verified before
// the move.
type fileID struct {
	dev uint64
	ino uint64
}

// storeLayout is one pinned view of the Store tree: the Store root and
// every internal directory are opened through os.Root and identity-verified
// one component at a time, so later mutations resolve names against pinned
// directory handles, never against paths an external writer could swap for
// symlinks. Every pinned handle retains its physical identity and the
// configured root path, so verify can re-prove that the logical paths still
// name the pinned objects before a public Store method reports success.
// The owning Store's deterministic test hook is retained so operations on
// the layout (such as removeOp) can expose the same narrow race seams.
type storeLayout struct {
	rootPath  string
	store     *os.Root
	internal  *os.Root
	staging   *os.Root
	baselines *os.Root
	previous  *os.Root
	recovery  *os.Root

	storeID     fileID
	internalID  fileID
	stagingID   fileID
	baselinesID fileID
	previousID  fileID
	recoveryID  fileID

	hook func(HookPoint)
}

// runHook invokes the layout's retained test seam at the given point, if
// set.
func (l *storeLayout) runHook(p HookPoint) {
	if l.hook != nil {
		l.hook(p)
	}
}

// openLayout pins the Store root and every internal directory. A symlinked,
// non-directory, or identity-swapped internal directory is refused, so a
// post-layout symlink swap can never redirect reads or mutations outside
// the Store. A missing layout is reported through os.ErrNotExist. The
// logical layout is re-proven before openLayout returns.
func (s Store) openLayout() (*storeLayout, error) {
	info, err := os.Lstat(s.Root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Skill Store root is not a directory")
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	rootID, err := rootID(root)
	if err != nil {
		root.Close()
		return nil, err
	}
	if fid, ferr := fileIDOf(info); ferr != nil || fid != rootID {
		root.Close()
		return nil, fmt.Errorf("Skill Store root changed while it was being opened")
	}
	l := &storeLayout{rootPath: s.Root, store: root, storeID: rootID, hook: s.hook}
	if err := l.openInternal(); err != nil {
		// Release every handle already pinned by openInternal, not only the
		// root, so a partial layout never leaks open FDs.
		l.close()
		return nil, err
	}
	if err := l.verify(); err != nil {
		l.close()
		return nil, err
	}
	return l, nil
}

func (l *storeLayout) openInternal() error {
	internal, internalID, err := openPinnedChild(l.store, ".skillctl")
	if err != nil {
		return err
	}
	l.internal = internal
	l.internalID = internalID
	for _, d := range []struct {
		name string
		dst  **os.Root
		id   *fileID
	}{
		{"staging", &l.staging, &l.stagingID},
		{"baselines", &l.baselines, &l.baselinesID},
		{"previous", &l.previous, &l.previousID},
		{"recovery", &l.recovery, &l.recoveryID},
	} {
		child, childID, err := openPinnedChild(internal, d.name)
		if err != nil {
			return err
		}
		*d.dst = child
		*d.id = childID
	}
	return nil
}

// close releases every pinned handle.
func (l *storeLayout) close() {
	for _, r := range []*os.Root{l.store, l.internal, l.staging, l.baselines, l.previous, l.recovery} {
		if r != nil {
			r.Close()
		}
	}
}

// openLayoutDir opens one single-component child directory below a pinned
// parent, refusing to follow a symlink and refusing a directory whose
// identity changed while it was being opened. os.ErrNotExist is returned
// when the child is absent.
func openLayoutDir(parent *os.Root, name string) (*os.Root, error) {
	child, _, err := openPinnedChild(parent, name)
	return child, err
}

// openPinnedChild opens the single-component directory name below the
// pinned parent and derives its identity from the already-open child
// handle: a symlink or non-directory is refused before opening, and the
// opened root must be the very object the parent entry named. A mutable
// pathname never supplies identity after an object was opened.
func openPinnedChild(parent *os.Root, name string) (*os.Root, fileID, error) {
	if err := validateRenameName(name); err != nil {
		return nil, fileID{}, err
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
	id, err := rootID(child)
	if err != nil {
		child.Close()
		return nil, fileID{}, err
	}
	if lid, lerr := fileIDOf(info); lerr != nil || lid != id {
		child.Close()
		return nil, fileID{}, fmt.Errorf("%s changed while it was being opened", name)
	}
	return child, id, nil
}

// rootID returns the physical identity of the already-open root handle.
func rootID(root *os.Root) (fileID, error) {
	st, err := root.Stat(".")
	if err != nil {
		return fileID{}, err
	}
	return fileIDOf(st)
}

// fileIDFromOpen returns the physical identity of an already-open file.
func fileIDFromOpen(f *os.File) (fileID, error) {
	st, err := f.Stat()
	if err != nil {
		return fileID{}, err
	}
	return fileIDOf(st)
}

// nameRefersTo reports whether the single-component name below the pinned
// parent still identifies the object with the given physical identity, so
// a pathname swap is detected after an object was opened.
func nameRefersTo(parent *os.Root, name string, expected fileID) error {
	info, err := parent.Lstat(name)
	if err != nil {
		return fmt.Errorf("%s is no longer present: %v", name, err)
	}
	if fid, ferr := fileIDOf(info); ferr != nil || fid != expected {
		return fmt.Errorf("%s no longer refers to the expected object", name)
	}
	return nil
}

// opDir pins the staging directory of one operation, or os.ErrNotExist
// when the operation has no staging.
func (l *storeLayout) opDir(opID int64) (*os.Root, error) {
	return openLayoutDir(l.staging, strconv.FormatInt(opID, 10))
}

// recoverySlot pins the recovery slot of one operation, or os.ErrNotExist
// when it does not exist.
func (l *storeLayout) recoverySlot(opID int64) (*os.Root, error) {
	return openLayoutDir(l.recovery, strconv.FormatInt(opID, 10))
}

// childExists reports whether name exists below the pinned parent.
func childExists(parent *os.Root, name string) (bool, error) {
	_, err := parent.Lstat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func fileIDOf(info os.FileInfo) (fileID, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}, fmt.Errorf("filesystem identity is unavailable for %s", info.Name())
	}
	return fileID{dev: uint64(st.Dev), ino: uint64(st.Ino)}, nil
}

// digestAt returns the canonical digest of the single-component directory
// name below a pinned parent, and whether it exists. The directory must be
// a real, identity-stable directory: a symlink or non-directory is
// rejected, never followed, and the digest comes from the opened handle.
func digestAt(ctx context.Context, parent *os.Root, name string) (string, bool, error) {
	child, err := openLayoutDir(parent, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	defer child.Close()
	digest, err := source.TreeDigestRoot(ctx, child)
	if err != nil {
		return "", false, err
	}
	return digest, true, nil
}

// openTreeDigest opens the single-component directory name below the pinned
// parent and returns its opened root, physical identity from that handle,
// and canonical digest — all derived from the same opened object, so a
// swapped name can never supply identity after hashing.
func openTreeDigest(ctx context.Context, parent *os.Root, name string) (*os.Root, fileID, string, error) {
	child, err := openLayoutDir(parent, name)
	if err != nil {
		return nil, fileID{}, "", err
	}
	id, err := rootID(child)
	if err != nil {
		child.Close()
		return nil, fileID{}, "", err
	}
	digest, err := source.TreeDigestRoot(ctx, child)
	if err != nil {
		child.Close()
		return nil, fileID{}, "", err
	}
	return child, id, digest, nil
}

// validateRenameName requires one path component, so a crafted or
// persisted-operation name can never traverse between pinned directories.
func validateRenameName(name string) error {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
		return fmt.Errorf("invalid single-component name %q", name)
	}
	return nil
}
