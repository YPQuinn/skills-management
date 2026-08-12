package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
)

// treeDigestHookPoint identifies one deterministic race window inside the
// rooted no-follow digest walk. The hook is package-private and nil in
// production; tests use it to swap a node between classification and open
// so the Stage-A invariants are exercised, not just assumed. It is a hook,
// not an interface: TreeDigestRoot is the only production caller.
type treeDigestHookPoint int

const (
	// hookRootFileBeforeOpen fires after a regular file was classified,
	// before its no-follow open.
	hookRootFileBeforeOpen treeDigestHookPoint = iota
	// hookRootDirBeforeRecurse fires after a directory was pinned open,
	// before its recursion re-reads the parent name.
	hookRootDirBeforeRecurse
)

var treeDigestRaceHook func(treeDigestHookPoint)

// TreeDigestRoot computes the canonical content digest of the directory
// tree rooted at the pinned handle root, using only single-component,
// no-follow, pinned access: directories are recursed through pinned child
// roots and regular files are hashed from an already-open no-follow handle
// with before/after identity checks, so a path swap inside the tree cannot
// change which content is hashed. The returned digest is identical to
// TreeDigest for the same content, so Store recovery and observation share
// one content identity.
func TreeDigestRoot(ctx context.Context, root *os.Root) (string, error) {
	var snap snapshot
	if err := snapshotTreeRoot(ctx, root, ".", &snap); err != nil {
		return "", err
	}
	return snapshotDigest(snap, "."), nil
}

// snapshotTreeRoot appends every node below the pinned root to snap, with
// paths relative to the tree root. Directories are recursed through pinned
// child roots and the parent entry is re-proven after each recursion;
// regular files are hashed from an already-open no-follow handle with
// identity, size, and mode re-checks; symlinks are recorded as symlink
// nodes and never opened. Source .git metadata is excluded exactly like
// the absolute-path snapshot walk.
func snapshotTreeRoot(ctx context.Context, root *os.Root, base string, snap *snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rootInfo, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("%s is not a directory", base)
	}
	snap.entries = append(snap.entries, snapshotEntry{
		relPath: base,
		kind:    kindDir,
		exec:    rootInfo.Mode()&0o111 != 0,
	})
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := e.Name()
		if name == ".git" {
			continue
		}
		childBase := path.Join(base, name)
		switch {
		case e.IsDir():
			if err := snapshotRootDir(ctx, root, name, childBase, snap); err != nil {
				return err
			}
		case e.Type().IsRegular():
			if err := snapshotRootFile(ctx, root, name, childBase, snap); err != nil {
				return err
			}
		case e.Type()&fs.ModeSymlink != 0:
			if err := snapshotRootLink(ctx, root, name, childBase, snap); err != nil {
				return err
			}
		default:
			snap.entries = append(snap.entries, snapshotEntry{
				relPath: childBase,
				kind:    snapshotKind(e.Type()),
			})
		}
	}
	return nil
}

// snapshotRootDir recurses into the single-component directory name below
// the pinned root through a pinned child root, then re-proves that the
// parent entry still identifies the recursed object.
func snapshotRootDir(ctx context.Context, root *os.Root, name, base string, snap *snapshot) error {
	child, childID, err := openPinnedDir(root, name)
	if err != nil {
		return err
	}
	if treeDigestRaceHook != nil {
		treeDigestRaceHook(hookRootDirBeforeRecurse)
	}
	err = snapshotTreeRoot(ctx, child, base, snap)
	child.Close()
	if err != nil {
		return err
	}
	// A directory swapped during the recursion is detected here: the
	// parent entry must still name the recursed child.
	if err := nameStillRefers(root, name, childID); err != nil {
		return fmt.Errorf("directory %q changed while it was being hashed: %v", base, err)
	}
	return nil
}

// snapshotRootFile hashes the single-component regular file name below the
// pinned root from an already-open no-follow handle: the parent Lstat must
// classify a regular file, the opened handle must be the same object, and
// after hashing both the handle and the parent entry must still identify
// the same regular file with the same size and mode.
func snapshotRootFile(ctx context.Context, root *os.Root, name, base string, snap *snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	linfo, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if linfo.Mode()&os.ModeSymlink != 0 || !linfo.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", base)
	}
	if treeDigestRaceHook != nil {
		treeDigestRaceHook(hookRootFileBeforeOpen)
	}
	f, err := openNoFollowFile(root, name)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(linfo, info) || !info.Mode().IsRegular() {
		return fmt.Errorf("file %q changed while it was being opened", base)
	}
	digest, err := hashReaderCtx(ctx, f)
	if err != nil {
		return err
	}
	after, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, after) || after.Size() != info.Size() || after.Mode() != info.Mode() || !after.Mode().IsRegular() {
		return fmt.Errorf("file %q changed while it was being hashed", base)
	}
	if err := nameStillRefers(root, name, fileIDOf(linfo)); err != nil {
		return fmt.Errorf("file %q was replaced while it was being hashed: %v", base, err)
	}
	snap.entries = append(snap.entries, snapshotEntry{
		relPath: base,
		kind:    kindFile,
		size:    info.Size(),
		exec:    info.Mode()&0o111 != 0,
		digest:  digest,
	})
	return nil
}

// snapshotRootLink records the target of the single-component symlink name
// below the pinned root, proving the parent entry still names the same
// symlink after the target was read.
func snapshotRootLink(ctx context.Context, root *os.Root, name, base string, snap *snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	linfo, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if linfo.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink", base)
	}
	target, err := root.Readlink(name)
	if err != nil {
		return err
	}
	if err := nameStillRefers(root, name, fileIDOf(linfo)); err != nil {
		return fmt.Errorf("symlink %q changed while its target was read: %v", base, err)
	}
	snap.entries = append(snap.entries, snapshotEntry{
		relPath:    base,
		kind:       kindSymlink,
		linkTarget: target,
	})
	return nil
}

// hashReaderCtx returns the hex SHA-256 of the reader's bytes, checking ctx
// between chunks so a cancelled observation aborts promptly even while
// hashing a large file.
func hashReaderCtx(ctx context.Context, r io.Reader) (string, error) {
	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, rerr := r.Read(buf)
		if n > 0 {
			if _, werr := h.Write(buf[:n]); werr != nil {
				return "", werr
			}
		}
		if rerr == io.EOF {
			return hex.EncodeToString(h.Sum(nil)), nil
		}
		if rerr != nil {
			return "", rerr
		}
	}
}
