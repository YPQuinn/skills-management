package source

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// testBeforeChmodFileHook and testBeforeChmodDirHook, when set, run after a
// rooted write created its file or directory and before the mode is applied
// through the still-open handle. Tests use them to swap the name for a
// symlink, proving the mode is never applied to the foreign target. Nil in
// production; tests must reset them.
var (
	testBeforeChmodFileHook func(parent *os.Root, name string)
	testBeforeChmodDirHook  func(parent *os.Root, name string)
)

// streamCopyRooted streams a large regular file from the already-open
// source into dst, honoring cancellation and removing a partial write on
// failure. The destination file is created relative to the pinned parent
// (every ancestor is created and pinned one component at a time) and the
// mode is applied through the still-open no-follow-created handle, so a
// mutable pathname is never re-traversed for the chmod. The pinned parent
// returned by ensureDirs is caller-owned and closed here when it is not
// the caller's own dst root.
func streamCopyRooted(ctx context.Context, in *os.File, dst *os.Root, rel string, mode os.FileMode) error {
	parent, err := ensureDirs(dst, rel)
	if err != nil {
		return err
	}
	if parent != dst {
		defer parent.Close()
	}
	name := path.Base(rel)
	out, err := parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			parent.Remove(name)
			return err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				parent.Remove(name)
				return werr
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			parent.Remove(name)
			return rerr
		}
	}
	if err := chmodOpenFile(parent, name, out, mode); err != nil {
		// The mode/identity check failed: close the created handle first
		// (best-effort, but the FD must never leak), then remove the
		// logical name, and keep the original chmod/name-mismatch error.
		out.Close()
		parent.Remove(name)
		return err
	}
	return out.Close()
}

// rootedWriteFile writes content into dst at rel with an exclusive create
// (never following a planted destination symlink), creating any missing
// parent directories through pinned handles first, and restoring the exact
// mode afterwards because umask may strip bits. The mode is applied through
// the still-open created handle — never through a mutable pathname. The
// pinned parent returned by ensureDirs is caller-owned and closed here when
// it is not the caller's own dst root.
func rootedWriteFile(ctx context.Context, dst *os.Root, rel string, content []byte, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parent, err := ensureDirs(dst, rel)
	if err != nil {
		return err
	}
	if parent != dst {
		defer parent.Close()
	}
	name := path.Base(rel)
	f, err := parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		parent.Remove(name)
		return err
	}
	if err := chmodOpenFile(parent, name, f, mode); err != nil {
		// The mode/identity check failed: close the created handle first
		// (best-effort, but the FD must never leak), then remove the
		// logical name, and keep the original chmod/name-mismatch error.
		f.Close()
		parent.Remove(name)
		return err
	}
	return f.Close()
}

// chmodOpenFile applies mode through the still-open file handle and then
// proves the parent entry still identifies the chmod'd object, so a symlink
// swapped onto the name after the create can never redirect the mode and is
// detected before the write reports success.
func chmodOpenFile(parent *os.Root, name string, f *os.File, mode os.FileMode) error {
	// Test seam: a symlink swapped in here must not redirect the mode: the
	// chmod runs on the open handle and the name re-proof below fails.
	if testBeforeChmodFileHook != nil {
		testBeforeChmodFileHook(parent, name)
	}
	if err := f.Chmod(mode); err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		return err
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return err
	}
	if !os.SameFile(st, info) {
		return fmt.Errorf("destination %q changed while its mode was set", name)
	}
	return nil
}

// ensureDirs creates every ancestor directory of rel below dst through
// pinned handles — each component is created and pinned one at a time
// relative to the previously pinned parent — and returns the pinned handle
// of the deepest ancestor, so a later single-component create or chmod
// never re-traverses a mutable path. Every intermediate root is closed as
// the descent proceeds and on every error path; the returned deepest root
// is caller-owned and must be closed by the caller (it is dst itself when
// rel has no directory component). A non-directory node is refused.
func ensureDirs(dst *os.Root, rel string) (*os.Root, error) {
	dir := path.Dir(rel)
	if dir == "." || dir == "" {
		return dst, nil
	}
	cur := dst
	for _, part := range strings.Split(dir, "/") {
		child, err := rootedMkdir(cur, part, 0o755)
		if err != nil {
			if cur != dst {
				cur.Close()
			}
			return nil, err
		}
		if cur != dst {
			cur.Close()
		}
		cur = child
	}
	return cur, nil
}

// rootedMkdir creates the single-component directory name below the pinned
// parent and returns it pinned: the child is opened without following a
// symlink and identity-verified against the parent entry, so a planted
// symlink is never used as a directory. A newly created directory has its
// mode set through the pinned child's own handle — fchmod on the child's
// root FD, never through a mutable pathname — and the logical name is
// re-proven to still identify the chmod'd object; an existing directory is
// never chmod'd at all. The returned child root is caller-owned and must
// be closed by the caller.
func rootedMkdir(parent *os.Root, name string, mode os.FileMode) (*os.Root, error) {
	err := parent.Mkdir(name, mode)
	if err == nil {
		child, _, err := openPinnedDir(parent, name)
		if err != nil {
			return nil, err
		}
		// Test seam: a symlink swapped in here must not redirect the mode:
		// the fchmod runs on the pinned child and the re-proof below fails.
		if testBeforeChmodDirHook != nil {
			testBeforeChmodDirHook(parent, name)
		}
		if err := chmodRootDir(child, mode); err != nil {
			child.Close()
			return nil, err
		}
		info, err := parent.Lstat(name)
		if err != nil {
			child.Close()
			return nil, err
		}
		st, err := child.Stat(".")
		if err != nil {
			child.Close()
			return nil, err
		}
		if !os.SameFile(info, st) {
			child.Close()
			return nil, fmt.Errorf("destination %q changed while its mode was set", name)
		}
		return child, nil
	}
	if os.IsExist(err) {
		child, _, err := openPinnedDir(parent, name)
		if err != nil {
			return nil, err
		}
		return child, nil
	}
	return nil, err
}

// chmodRootDir applies mode to the pinned directory root through its own
// handle: the root's directory FD is opened and fchmod'd, so no mutable
// pathname is ever re-traversed and a symlink swap can never redirect the
// mode change to a foreign target.
func chmodRootDir(child *os.Root, mode os.FileMode) error {
	f, err := child.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Chmod(mode)
}
