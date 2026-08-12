package skillstore

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"syscall"

	"skillctl/internal/domain"
)

// testBeforeChmodDirHook and testBeforeChmodFileHook are the narrow
// package-private seams for the pinned-mode invariants: they run after a
// directory or file was created and pinned, immediately before its mode is
// applied through the pinned handle. Tests use them to swap the logical
// name for a symlink, proving the mode is never applied to the foreign
// target. Nil in production; tests must reset them.
var (
	testBeforeChmodDirHook  func(parent *os.Root, name string)
	testBeforeChmodFileHook func(parent *os.Root, name string)
)

// copyStagedTree copies one tree rooted at src (srcRel "." is its root)
// into the pinned dst root, applying the per-Skill guards and re-validating
// every node: each node is classified by the opened directory's entry type
// and re-checked on the opened file, so a symlink, FIFO, socket, or device
// planted into the source or staging trees is rejected even with allowLarge
// and never followed. Every destination directory is created exclusively
// and pinned one component at a time and every file is written with an
// exclusive create relative to the pinned parent, so a planted symlink can
// never be written through and no mutable path is ever re-traversed after a
// node was pinned. Every created node is recorded in the invocation's
// ownership manifest under dstRel (relative to the staging root), so a
// partial copy is removed only through the recorded identities. .git nodes
// of any type are skipped like every other Store-tree copy.
func copyStagedTree(ctx context.Context, src *os.Root, srcRel string, dst *os.Root, dstRel string, guard *domain.Guard, own *stageOwnership) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := src.Open(srcRel)
	if err != nil {
		return err
	}
	info, err := dir.Stat()
	if err != nil {
		dir.Close()
		return err
	}
	if !info.IsDir() {
		dir.Close()
		return fmt.Errorf("%s is not a directory", srcRel)
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
		switch {
		case e.IsDir():
			childInfo, err := e.Info()
			if err != nil {
				return err
			}
			child, childID, err := createPinnedDirExclusive(dst, name, childInfo.Mode().Perm())
			if err != nil {
				return err
			}
			own.record(path.Join(dstRel, name), childID)
			err = copyStagedTree(ctx, src, path.Join(srcRel, name), child, path.Join(dstRel, name), guard, own)
			child.Close()
			if err != nil {
				return err
			}
		case e.Type().IsRegular():
			if err := copyStagedFile(ctx, src, path.Join(srcRel, name), dst, dstRel, name, guard, own); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s is a %s; special nodes cannot be imported", path.Join(srcRel, name), nodeKindName(e.Type()))
		}
	}
	return nil
}

// copyStagedFile copies one regular file from the already-open-verified
// source into the pinned parent root, checking the per-Skill guards against
// the opened node's size, writing through an exclusive create relative to
// the pinned parent, and honoring cancellation. The created file is
// recorded in the ownership manifest immediately, so a partial or refused
// copy is removed by the manifest cleanup, never by a fresh sample. The
// mode is applied through the still-open no-follow-created handle — never
// through a mutable pathname — and the parent entry must still identify the
// chmod'd object before the handle is closed.
func copyStagedFile(ctx context.Context, src *os.Root, srcRel string, dst *os.Root, dstRel, name string, guard *domain.Guard, own *stageOwnership) error {
	in, err := src.OpenFile(srcRel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", srcRel)
	}
	if err := guard.AddFile(info.Size()); err != nil {
		return err
	}
	out, err := dst.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	fid, err := fileIDFromOpen(out)
	if err != nil {
		out.Close()
		return err
	}
	own.record(path.Join(dstRel, name), fid)
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			return err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return werr
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return rerr
		}
	}
	// Test seam: a symlink swapped in here must not redirect the mode: the
	// chmod runs on the open handle and the name re-proof below fails.
	if testBeforeChmodFileHook != nil {
		testBeforeChmodFileHook(dst, name)
	}
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		out.Close()
		return err
	}
	st, err := out.Stat()
	if err != nil {
		out.Close()
		return err
	}
	linfo, err := dst.Lstat(name)
	if err != nil {
		out.Close()
		return err
	}
	if !os.SameFile(st, linfo) {
		out.Close()
		return fmt.Errorf("destination %q changed while its mode was set", name)
	}
	return out.Close()
}

// nodeKindName names a non-regular, non-directory node for errors.
func nodeKindName(m fs.FileMode) string {
	switch {
	case m&fs.ModeSymlink != 0:
		return "symlink"
	case m&fs.ModeNamedPipe != 0:
		return "FIFO"
	case m&fs.ModeSocket != 0:
		return "socket"
	case m&fs.ModeDevice != 0:
		return "device"
	default:
		return "special node"
	}
}
