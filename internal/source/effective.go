package source

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// effNode is one node of a Skill tree in the effective materialized form:
// the form the Store actually holds. Regular files and directories are
// themselves; a symlink keeps only its target string and is dereferenced
// during digest computation, exactly as materialization dereferences it
// into the Store copy.
type effNode struct {
	kind    string // canonKind* values
	exec    bool
	content string // file content digest, or symlink target
}

// snapshotEffNodes builds the effective node map of one Skill from the raw
// observation snapshot, mapping entries relative to base (the Skill's
// relative directory; "." for the root skill) to their entry-relative
// paths, exactly like snapshotDigest.
func snapshotEffNodes(snap snapshot, base string) map[string]effNode {
	nodes := map[string]effNode{}
	for _, e := range snap.entries {
		rel := e.relPath
		if base != "." {
			if rel == base {
				rel = "."
			} else if strings.HasPrefix(rel, base+"/") {
				rel = strings.TrimPrefix(rel, base+"/")
			} else {
				continue
			}
		}
		n := effNode{kind: string(e.kind)}
		switch e.kind {
		case kindFile:
			n.exec = e.exec
			n.content = e.digest
		case kindSymlink:
			n.content = e.linkTarget
		}
		nodes[rel] = n
	}
	return nodes
}

// snapshotEffectiveDigest is the effective materialized digest of one
// Skill's observation snapshot: safe internal symlinks are dereferenced, so
// the digest matches the self-contained Store copy materialization
// produces. A symlink that is broken, escapes the Skill root, loops, or
// resolves to a special node makes the whole Skill invalid (the caller
// records it as an inventory Issue).
func snapshotEffectiveDigest(snap snapshot, base string) (string, error) {
	return effectiveDigest(snapshotEffNodes(snap, base))
}

// effectiveDigest computes the canonical digest of the effective tree:
// every symlink node is resolved within the tree (chains included) and
// replaced by the file or directory subtree it names. The digest is exactly
// the digest of the dereferenced Store copy, so observation, materialized
// content, Store live trees, and Baselines share one content identity.
func effectiveDigest(nodes map[string]effNode) (string, error) {
	var out []canonNode
	visiting := map[string]bool{}
	if err := emitEffective(nodes, ".", ".", visiting, &out); err != nil {
		return "", err
	}
	return canonDigest(out), nil
}

// emitEffective appends the effective nodes for the tree at lookup (a
// path in the entry's path space) into out, emitting each node at emitPath
// (the entry-relative path the materialized copy will use). The visiting
// set detects cycles through symlink chains and links to ancestor
// directories.
func emitEffective(nodes map[string]effNode, lookup, emitPath string, visiting map[string]bool, out *[]canonNode) error {
	n, ok := nodes[lookup]
	if !ok {
		return fmt.Errorf("symlink target %q is missing", lookup)
	}
	switch n.kind {
	case canonKindFile:
		*out = append(*out, canonNode{relPath: emitPath, kind: canonKindFile, exec: n.exec, content: n.content})
		return nil
	case canonKindDir:
		if visiting[lookup] {
			return fmt.Errorf("symlink loop detected at %q", lookup)
		}
		visiting[lookup] = true
		defer delete(visiting, lookup)
		*out = append(*out, canonNode{relPath: emitPath, kind: canonKindDir})
		for _, child := range sortedChildren(nodes, lookup) {
			childEmit := path.Join(emitPath, strings.TrimPrefix(child, lookup+"/"))
			if err := emitEffective(nodes, child, childEmit, visiting, out); err != nil {
				return err
			}
		}
		return nil
	case canonKindSymlink:
		if visiting[lookup] {
			return fmt.Errorf("symlink loop detected at %q", lookup)
		}
		visiting[lookup] = true
		defer delete(visiting, lookup)
		target := n.content
		if strings.HasPrefix(target, "/") {
			return fmt.Errorf("symlink %q escapes the Skill root", lookup)
		}
		resolved := path.Clean(path.Join(path.Dir(lookup), target))
		if resolved == "." {
			// the link names the Skill root itself, so its dereferenced
			// copy would contain the link forever
			return fmt.Errorf("symlink loop detected at %q", lookup)
		}
		if resolved == ".." || strings.HasPrefix(resolved, "../") {
			return fmt.Errorf("symlink %q escapes the Skill root", lookup)
		}
		if resolved == lookup {
			return fmt.Errorf("symlink loop detected at %q", lookup)
		}
		return emitEffective(nodes, resolved, emitPath, visiting, out)
	default:
		return fmt.Errorf("%s is a %s and cannot be imported", lookup, n.kind)
	}
}

// sortedChildren returns the immediate children of dir (keys with exactly
// one path segment below dir), sorted for deterministic digests. The root
// is ".", whose children are the top-level keys without any separator.
func sortedChildren(nodes map[string]effNode, dir string) []string {
	var out []string
	for k := range nodes {
		if k == dir {
			continue
		}
		if dir == "." {
			if !strings.Contains(k, "/") {
				out = append(out, k)
			}
			continue
		}
		if rest := strings.TrimPrefix(k, dir+"/"); rest != k && !strings.Contains(rest, "/") {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
