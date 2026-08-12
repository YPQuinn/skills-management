package source

import (
	"fmt"
	"strings"
)

// validateRelPath rejects tree-relative paths that are absolute, empty, or
// non-canonical (".", "..", or empty segments). Git refuses to track such
// paths, but a crafted repository must never make a destination write
// escape its root, so every path materialization touches is validated
// first.
func validateRelPath(rel string) error {
	if rel == "" || strings.HasPrefix(rel, "/") {
		return fmt.Errorf("invalid tree path %q", rel)
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid tree path %q", rel)
		}
	}
	return nil
}

// gitRelativePath converts one ls-tree output path to the Source-subpath
// namespace. The output must actually lie below that subpath and be a
// canonical relative path before discovery or destination access uses it.
func gitRelativePath(repoPath, subpath string) (string, error) {
	rel := repoPath
	if subpath != "" {
		prefix := subpath + "/"
		if !strings.HasPrefix(repoPath, prefix) {
			return "", fmt.Errorf("tree path %q is outside Source subpath %q", repoPath, subpath)
		}
		rel = strings.TrimPrefix(repoPath, prefix)
	}
	if err := validateRelPath(rel); err != nil {
		return "", err
	}
	return rel, nil
}

// gitEffNodes builds the effective node map of one Git Skill entry from its
// tree rows and blobs. Directory nodes are implicit and synthesized from
// file paths (git does not track empty directories), so the map matches the
// Local snapshot's node map for equivalent content. Non-canonical paths are
// rejected so a crafted tree cannot alias or escape.
func gitEffNodes(rows []treeLine, dir string, blobs map[string][]byte) (map[string]effNode, error) {
	nodes := map[string]effNode{}
	seenDirs := map[string]bool{}
	addDirs := func(fileRel string) {
		d := fileRel
		for {
			if i := strings.LastIndex(d, "/"); i < 0 {
				d = "."
			} else {
				d = d[:i]
			}
			if !seenDirs[d] {
				seenDirs[d] = true
				nodes[d] = effNode{kind: canonKindDir}
			}
			if d == "." {
				return
			}
		}
	}
	for _, l := range rows {
		rel := l.rel
		if dir != "" {
			rel = strings.TrimPrefix(rel, dir+"/")
		}
		if err := validateRelPath(rel); err != nil {
			return nil, err
		}
		addDirs(rel)
		n := effNode{}
		switch {
		case l.mode == "100644":
			n.kind = canonKindFile
			n.content = hashBytes(blobs[l.oid])
		case l.mode == "100755":
			n.kind = canonKindFile
			n.exec = true
			n.content = hashBytes(blobs[l.oid])
		case l.mode == "120000":
			n.kind = canonKindSymlink
			n.content = string(blobs[l.oid])
		case l.mode == "160000":
			n.kind = canonKindGitlink
			n.content = l.oid
		default:
			n.kind = canonKindOther
			n.content = l.oid
		}
		nodes[rel] = n
	}
	return nodes, nil
}

// skillTreeEffectiveDigest is the effective materialized digest of one Git
// Skill entry: safe internal symlinks are dereferenced exactly as
// materialization dereferences them, so the digest equals both the Local
// observation digest of equivalent content and the digest of the Store
// copy. A broken, escaping, looping, gitlink, or special symlink target
// rejects the whole Skill.
func skillTreeEffectiveDigest(rows []treeLine, dir string, blobs map[string][]byte) (string, error) {
	nodes, err := gitEffNodes(rows, dir, blobs)
	if err != nil {
		return "", err
	}
	return effectiveDigest(nodes)
}
