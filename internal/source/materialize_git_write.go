package source

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"skillctl/internal/domain"
)

// writeGitTree materializes the rows of one Skill entry into dst: regular
// files keep their content and executable bit, symlinks are dereferenced to
// their in-tree target, and gitlinks and unknown modes reject the Skill.
// Every write is rooted and exclusive so a planted destination symlink is
// never followed; the per-Skill guards are re-checked as defense.
func writeGitTree(ctx context.Context, rows []treeLine, dir string, blobs map[string][]byte, dst *os.Root, guard *domain.Guard) error {
	nodes := map[string]treeLine{}
	for _, r := range rows {
		rel := r.rel
		if dir != "" {
			rel = strings.TrimPrefix(rel, dir+"/")
		}
		if isDotGitPath(rel) {
			continue
		}
		nodes[rel] = r
	}
	paths := make([]string, 0, len(nodes))
	for p := range nodes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	visiting := map[string]bool{}
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeGitNode(ctx, nodes, blobs, p, p, dst, guard, visiting); err != nil {
			return err
		}
	}
	return nil
}

// writeGitNode writes the node at rel into dst at dstRel (usually the same
// path; a dereferenced symlink writes its target content at the link's own
// path), rejecting gitlinks and unknown modes.
func writeGitNode(ctx context.Context, nodes map[string]treeLine, blobs map[string][]byte, rel, dstRel string, dst *os.Root, guard *domain.Guard, visiting map[string]bool) error {
	r, ok := nodes[rel]
	if !ok {
		return fmt.Errorf("entry path %q is missing from the tree", rel)
	}
	switch {
	case r.mode == "100644" || r.mode == "100755":
		return writeGitFile(ctx, blobs, r, dstRel, dst, guard)
	case r.mode == "120000":
		resolved := gitResolveTarget(rel, string(blobs[r.oid]))
		if resolved == "" {
			return fmt.Errorf("symlink %q escapes the Skill root", rel)
		}
		return writeGitResolved(ctx, nodes, blobs, resolved, dstRel, dst, guard, visiting)
	case r.mode == "160000":
		return fmt.Errorf("%s is a gitlink (submodule content is not materialized); import the submodule content separately", dstRel)
	default:
		return fmt.Errorf("unsupported tree entry mode %s at %q", r.mode, dstRel)
	}
}

// writeGitResolved writes the content that a symlink resolves to into dst
// at the link's own path: a file's content, a directory's whole subtree, or
// a further dereference through another symlink. A target that is missing,
// that escapes the entry tree, or that forms a link loop rejects the Skill.
func writeGitResolved(ctx context.Context, nodes map[string]treeLine, blobs map[string][]byte, resolved, dstRel string, dst *os.Root, guard *domain.Guard, visiting map[string]bool) error {
	if r, ok := nodes[resolved]; ok {
		switch {
		case r.mode == "100644" || r.mode == "100755":
			return writeGitFile(ctx, blobs, r, dstRel, dst, guard)
		case r.mode == "120000":
			next := gitResolveTarget(resolved, string(blobs[r.oid]))
			if next == "" {
				return fmt.Errorf("symlink %q escapes the Skill root", resolved)
			}
			if visiting[next] {
				return fmt.Errorf("symlink loop detected at %q", resolved)
			}
			visiting[next] = true
			err := writeGitResolved(ctx, nodes, blobs, next, dstRel, dst, guard, visiting)
			delete(visiting, next)
			return err
		case r.mode == "160000":
			return fmt.Errorf("%s is a gitlink (submodule content is not materialized); import the submodule content separately", resolved)
		}
	}
	// the resolved path is a directory: copy its whole subtree
	if visiting[resolved] {
		return fmt.Errorf("symlink loop detected at %q", resolved)
	}
	visiting[resolved] = true
	err := writeGitDirChildren(ctx, nodes, blobs, resolved, dstRel, dst, guard, visiting)
	delete(visiting, resolved)
	return err
}

// writeGitDirChildren writes every node under dir into dst below dstRel,
// so a dereferenced directory becomes a self-contained copy. The directory
// itself is created first so an empty target still materializes. The
// pinned handles opened for the creation — the deepest ancestor returned
// by ensureDirs and the created directory itself — are released
// immediately afterwards: every child write re-pins the ancestors rooted
// at dst, so no handle outlives its own step and a deep tree cannot
// accumulate open FDs.
func writeGitDirChildren(ctx context.Context, nodes map[string]treeLine, blobs map[string][]byte, dir, dstRel string, dst *os.Root, guard *domain.Guard, visiting map[string]bool) error {
	// The directory itself and every ancestor are created through pinned
	// handles, so an empty target still materializes and every child write
	// stays rooted at the pinned parent.
	parent, err := ensureDirs(dst, dstRel)
	if err != nil {
		return err
	}
	child, err := rootedMkdir(parent, path.Base(dstRel), 0o755)
	if parent != dst {
		parent.Close()
	}
	if err != nil {
		return err
	}
	child.Close()
	prefix := dir + "/"
	var children []string
	for p := range nodes {
		if strings.HasPrefix(p, prefix) {
			children = append(children, p)
		}
	}
	if len(children) == 0 {
		return fmt.Errorf("symlink target %q is missing from the tree", dir)
	}
	sort.Strings(children)
	for _, c := range children {
		if err := ctx.Err(); err != nil {
			return err
		}
		dstChild := path.Join(dstRel, strings.TrimPrefix(c, prefix))
		if err := writeGitNode(ctx, nodes, blobs, c, dstChild, dst, guard, visiting); err != nil {
			return err
		}
	}
	return nil
}

// writeGitFile writes one blob to dst at dstRel with the entry's executable
// mode, rejecting a canonical Git LFS pointer and re-checking the guards.
func writeGitFile(ctx context.Context, blobs map[string][]byte, r treeLine, dstRel string, dst *os.Root, guard *domain.Guard) error {
	content, ok := blobs[r.oid]
	if !ok {
		return fmt.Errorf("blob %s for %q is missing", r.oid, dstRel)
	}
	if isLFSPointer(content) {
		return lfsPointerError(dstRel)
	}
	if err := guard.AddFile(int64(len(content))); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if r.mode == "100755" {
		mode = 0o755
	}
	return rootedWriteFile(ctx, dst, dstRel, content, mode)
}

// gitResolveTarget resolves a symlink target relative to its own directory
// and returns the cleaned in-tree path, or "" when the target is absolute
// or escapes the entry tree.
func gitResolveTarget(linkPath, target string) string {
	if strings.HasPrefix(target, "/") {
		return ""
	}
	resolved := path.Clean(path.Join(path.Dir(linkPath), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return ""
	}
	return resolved
}
