package source

import (
	"context"
	"fmt"
	"os"
	"path"
	"syscall"

	"skillctl/internal/domain"
)

// preflightRootTree validates and counts the effective dereferenced Local
// tree before destination creation. The same rooted source is walked again
// for copying, where every opened file is rechecked against a fresh Guard.
func preflightRootTree(ctx context.Context, src *os.Root, rel string, guard *domain.Guard, visiting map[fileID]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := src.Open(rel)
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
		return fmt.Errorf("Skill contains a non-directory at %q", rel)
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
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name() == ".git" {
			continue
		}
		child := path.Join(rel, entry.Name())
		switch {
		case entry.IsDir():
			if err := preflightRootTree(ctx, src, child, guard, visiting); err != nil {
				return err
			}
		case entry.Type().IsRegular():
			if err := preflightRootFile(src, child, guard); err != nil {
				return err
			}
		case entry.Type()&os.ModeSymlink != 0:
			if err := preflightRootLink(ctx, src, child, guard, visiting); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Skill contains a special node at %s, which cannot be imported", child)
		}
	}
	return nil
}

func preflightRootLink(ctx context.Context, src *os.Root, linkRel string, guard *domain.Guard, visiting map[fileID]bool) error {
	resolved, info, err := rootedLinkTarget(src, linkRel)
	if err != nil {
		return err
	}
	switch {
	case info.IsDir():
		return preflightRootTree(ctx, src, resolved, guard, visiting)
	case info.Mode().IsRegular():
		return preflightRootFile(src, resolved, guard)
	default:
		return fmt.Errorf("symlink %s resolves to a special node, which cannot be imported", linkRel)
	}
}

func preflightRootFile(src *os.Root, rel string, guard *domain.Guard) error {
	file, err := src.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	file.Close()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", rel)
	}
	return guard.AddFile(info.Size())
}
