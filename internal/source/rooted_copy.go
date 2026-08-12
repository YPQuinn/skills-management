package source

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"skillctl/internal/domain"
)

// fileID is the (device, inode) identity of one real directory, used to
// detect symlink cycles that string paths alone cannot: a link resolving to
// an ancestor through a different alias would otherwise descend forever.
type fileID struct {
	dev uint64
	ino uint64
}

func fileIDOf(info os.FileInfo) fileID {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}
	}
	return fileID{dev: uint64(st.Dev), ino: uint64(st.Ino)}
}

// copyRootTree copies the directory at srcRel ("." for the Skill root) into
// the pinned dst root, dereferencing internal symlinks, rejecting
// broken/escaping/looping links and special nodes, preserving executable
// bits, skipping .git nodes of any type, and applying the per-Skill guards
// before each file's bytes are read. Every read is rooted at src and every
// write at the pinned dst — every child directory is created and pinned one
// component at a time — so neither side can escape and no mutable path is
// re-traversed after a node was pinned.
func copyRootTree(ctx context.Context, src *os.Root, srcRel string, dst *os.Root, guard *domain.Guard, visiting map[fileID]bool) error {
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
		return fmt.Errorf("Skill contains a non-directory at %q", srcRel)
	}
	id := fileIDOf(info)
	if visiting[id] {
		dir.Close()
		return fmt.Errorf("symlink loop detected while importing")
	}
	visiting[id] = true
	defer delete(visiting, id)
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
		childSrc := path.Join(srcRel, name)
		switch {
		case e.IsDir():
			child, err := rootedMkdir(dst, name, 0o755)
			if err != nil {
				return err
			}
			err = copyRootTree(ctx, src, childSrc, child, guard, visiting)
			child.Close()
			if err != nil {
				return err
			}
		case e.Type().IsRegular():
			if err := copyRootedFile(ctx, src, childSrc, dst, name, guard); err != nil {
				return err
			}
		case e.Type()&os.ModeSymlink != 0:
			if err := copyRootedLink(ctx, src, childSrc, dst, name, guard, visiting); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Skill contains a special node at %s, which cannot be imported", childSrc)
		}
	}
	return nil
}

// copyRootedLink dereferences one symlink into dst at the link's own path:
// the target's content is copied in place for a file, or its whole subtree
// for a directory, so the materialized tree contains no symlinks. Absolute,
// escaping, broken, and looping targets reject the Skill.
func copyRootedLink(ctx context.Context, src *os.Root, linkRel string, dst *os.Root, dstRel string, guard *domain.Guard, visiting map[fileID]bool) error {
	resolved, info, err := rootedLinkTarget(src, linkRel)
	if err != nil {
		return err
	}
	switch {
	case info.IsDir():
		// Materialize the link path as a pinned directory before
		// recursing, so an empty target directory still produces the
		// directory in dst and every child write stays rooted at it.
		child, err := rootedMkdir(dst, dstRel, 0o755)
		if err != nil {
			return err
		}
		defer child.Close()
		return copyRootTree(ctx, src, resolved, child, guard, visiting)
	case info.Mode().IsRegular():
		return copyRootedFile(ctx, src, resolved, dst, dstRel, guard)
	default:
		return fmt.Errorf("symlink %s resolves to a special node, which cannot be imported", linkRel)
	}
}

// rootedLinkTarget resolves a symlink within src and stats its target.
// os.Root rejects absolute and escaping chains even if a node changes while
// resolution is in progress.
func rootedLinkTarget(src *os.Root, linkRel string) (string, os.FileInfo, error) {
	target, err := src.Readlink(linkRel)
	if err != nil {
		return "", nil, err
	}
	if filepath.IsAbs(target) {
		return "", nil, fmt.Errorf("symlink %s escapes the Skill root", linkRel)
	}
	resolved := path.Clean(path.Join(path.Dir(linkRel), target))
	if resolved == "." {
		return "", nil, fmt.Errorf("symlink loop detected at %q", linkRel)
	}
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return "", nil, fmt.Errorf("symlink %s escapes the Skill root", linkRel)
	}
	info, err := src.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("broken symlink %s: %v", linkRel, err)
	}
	return resolved, info, nil
}

// copyRootedFile copies one regular file rooted at src into dst, checking
// the per-Skill guards against the opened node's size before any byte is
// read, rejecting a canonical Git LFS pointer, and writing through an
// exclusive create so a planted destination symlink is never followed.
func copyRootedFile(ctx context.Context, src *os.Root, srcRel string, dst *os.Root, dstRel string, guard *domain.Guard) error {
	f, err := src.OpenFile(srcRel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", srcRel)
	}
	if err := guard.AddFile(info.Size()); err != nil {
		return err
	}
	if info.Size() <= materializeBuffer {
		data, err := io.ReadAll(io.LimitReader(f, materializeBuffer+1))
		if err != nil {
			return err
		}
		if isLFSPointer(data) {
			return lfsPointerError(srcRel)
		}
		return rootedWriteFile(ctx, dst, dstRel, data, info.Mode().Perm())
	}
	return streamCopyRooted(ctx, f, dst, dstRel, info.Mode().Perm())
}
