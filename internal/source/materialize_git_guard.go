package source

import (
	"fmt"
	"sort"
	"strings"

	"skillctl/internal/domain"
)

// preflightGitMaterializedGuards counts the exact output tree, including
// files duplicated by dereferenced symlinks, using ls-tree sizes before any
// regular-file blob is fetched.
func preflightGitMaterializedGuards(rows []treeLine, dir string, linkBlobs map[string][]byte) error {
	nodes := make(map[string]treeLine, len(rows))
	for _, row := range rows {
		rel := row.rel
		if dir != "" {
			rel = strings.TrimPrefix(rel, dir+"/")
		}
		nodes[rel] = row
	}
	paths := make([]string, 0, len(nodes))
	for rel := range nodes {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	guard := &domain.Guard{}
	for _, rel := range paths {
		if err := preflightGitNode(nodes, linkBlobs, rel, guard, map[string]bool{}); err != nil {
			return err
		}
	}
	return nil
}

func preflightGitNode(nodes map[string]treeLine, linkBlobs map[string][]byte, rel string, guard *domain.Guard, visiting map[string]bool) error {
	row, ok := nodes[rel]
	if !ok {
		return fmt.Errorf("entry path %q is missing from the tree", rel)
	}
	switch row.mode {
	case "100644", "100755":
		if row.size < 0 {
			return fmt.Errorf("blob size for %q is unavailable", rel)
		}
		return guard.AddFile(row.size)
	case "120000":
		target, ok := linkBlobs[row.oid]
		if !ok {
			return fmt.Errorf("symlink target for %q is missing", rel)
		}
		resolved := gitResolveTarget(rel, string(target))
		if resolved == "" {
			return fmt.Errorf("symlink %q escapes the Skill root", rel)
		}
		return preflightGitResolved(nodes, linkBlobs, resolved, guard, visiting)
	case "160000":
		return fmt.Errorf("%s is a gitlink (submodule content is not materialized); import the submodule content separately", rel)
	default:
		return fmt.Errorf("unsupported tree entry mode %s at %q", row.mode, rel)
	}
}

func preflightGitResolved(nodes map[string]treeLine, linkBlobs map[string][]byte, resolved string, guard *domain.Guard, visiting map[string]bool) error {
	if visiting[resolved] {
		return fmt.Errorf("symlink loop detected at %q", resolved)
	}
	visiting[resolved] = true
	defer delete(visiting, resolved)
	if row, ok := nodes[resolved]; ok {
		switch row.mode {
		case "100644", "100755":
			if row.size < 0 {
				return fmt.Errorf("blob size for %q is unavailable", resolved)
			}
			return guard.AddFile(row.size)
		case "120000":
			target, ok := linkBlobs[row.oid]
			if !ok {
				return fmt.Errorf("symlink target for %q is missing", resolved)
			}
			next := gitResolveTarget(resolved, string(target))
			if next == "" {
				return fmt.Errorf("symlink %q escapes the Skill root", resolved)
			}
			return preflightGitResolved(nodes, linkBlobs, next, guard, visiting)
		case "160000":
			return fmt.Errorf("%s is a gitlink (submodule content is not materialized); import the submodule content separately", resolved)
		default:
			return fmt.Errorf("unsupported tree entry mode %s at %q", row.mode, resolved)
		}
	}
	prefix := resolved + "/"
	var children []string
	for rel := range nodes {
		if strings.HasPrefix(rel, prefix) {
			children = append(children, rel)
		}
	}
	if len(children) == 0 {
		return fmt.Errorf("symlink target %q is missing from the tree", resolved)
	}
	sort.Strings(children)
	for _, child := range children {
		if err := preflightGitNode(nodes, linkBlobs, child, guard, visiting); err != nil {
			return err
		}
	}
	return nil
}

func gitSymlinkRows(rows []treeLine) []treeLine {
	links := make([]treeLine, 0)
	for _, row := range rows {
		if row.mode == "120000" {
			links = append(links, row)
		}
	}
	return links
}
