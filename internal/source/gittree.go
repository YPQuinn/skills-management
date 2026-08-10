package source

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// treeLine is one ls-tree row parsed into its canonical fields: mode,
// object type, object id, the repository path, and the path relative to the
// Source subpath root.
type treeLine struct {
	mode string
	typ  string
	oid  string
	path string
	rel  string
}

// discoverCommit validates the Source subpath at the commit, lists the tree
// under it with NUL-safe plumbing, and applies the same depth-three
// discovery rules as Discover. Every entry carries the canonical digest of
// its complete tree; the Observation carries the aggregate Inventory
// digest. The locator identifies the remote so blob fetches reuse the same
// ambient credential helper as every other git invocation.
func discoverCommit(ctx context.Context, cache, commit string, loc Locator) (Observation, error) {
	if err := validateSubpathTree(ctx, cache, commit, loc.Subpath); err != nil {
		return Observation{}, err
	}
	args := []string{"-C", cache, "ls-tree", "-z", "-r", commit}
	if loc.Subpath != "" {
		args = append(args, "--", loc.Subpath)
	}
	out, err := runGit(ctx, args...)
	if err != nil {
		return Observation{}, fmt.Errorf("reading tree %s: %v", commit, err)
	}
	rows, err := parseTreeRecords(out)
	if err != nil {
		return Observation{}, err
	}

	prefix := ""
	if loc.Subpath != "" {
		prefix = loc.Subpath + "/"
	}
	all := make([]treeLine, 0, len(rows))
	// relative Skill directory -> its SKILL.md listing row
	skills := map[string]treeLine{}
	for _, r := range rows {
		r.rel = strings.TrimPrefix(r.path, prefix)
		all = append(all, r)
		if !strings.HasSuffix(r.rel, "/SKILL.md") && r.rel != "SKILL.md" {
			continue
		}
		dir := "."
		if r.rel != "SKILL.md" {
			dir = strings.TrimSuffix(r.rel, "/SKILL.md")
		}
		if _, exists := skills[dir]; !exists {
			skills[dir] = r
		}
	}

	var obs Observation
	// A SKILL.md directly in the subpath root is the whole Inventory.
	if marker, ok := skills["."]; ok {
		if err := obs.addSkill(ctx, cache, all, marker, "", loc); err != nil {
			return Observation{}, err
		}
		obs.Digest = inventoryDigest(obs.Entries, obs.Issues)
		return obs, nil
	}

	var dirs []string
	for dir := range skills {
		if dir == "." || isNestedSkillDir(dir, skills) || hasSkippedSegment(dir) {
			continue
		}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		if len(strings.Split(dir, "/")) > 3 {
			continue
		}
		if err := obs.addSkill(ctx, cache, all, skills[dir], dir, loc); err != nil {
			return Observation{}, err
		}
	}
	obs.Digest = inventoryDigest(obs.Entries, obs.Issues)
	return obs, nil
}

// validateSubpathTree ensures the Source subpath names a tree at the chosen
// commit, so a missing or file subpath fails the check instead of appearing
// as a successful empty Inventory. The subpath root itself always exists.
func validateSubpathTree(ctx context.Context, cache, commit, subpath string) error {
	if subpath == "" {
		return nil
	}
	typ, err := runGit(ctx, "-C", cache, "cat-file", "-t", commit+":"+subpath)
	if err != nil {
		return fmt.Errorf("subpath %q does not exist in the repository at %s", subpath, commit)
	}
	if strings.TrimSpace(typ) != "tree" {
		return fmt.Errorf("subpath %q is not a directory in the repository at %s", subpath, commit)
	}
	return nil
}

// addSkill validates one Skill candidate: it fetches the blobs of its tree,
// computes the canonical digest, validates the SKILL.md marker, and records
// an Entry or an Issue. dir is "" for the whole-subpath root Skill (whose
// entries are recorded as ".") and the relative directory otherwise. loc
// carries the remote identity for the blob fetch's credential helper.
func (obs *Observation) addSkill(ctx context.Context, cache string, all []treeLine, marker treeLine, dir string, loc Locator) error {
	rows := skillRows(all, dir)
	blobs, err := fetchTreeBlobs(ctx, cache, rows, loc)
	if err != nil {
		return fmt.Errorf("reading Skill tree: %v", err)
	}
	digest := skillTreeDigest(rows, dir, blobs)
	relDir := dir
	if relDir == "" {
		relDir = "."
	}
	data, ok := blobs[marker.oid]
	obs.addSkillBlob(marker, relDir, digest, data, ok)
	return nil
}

// skillRows returns the rows belonging to one Skill: the whole subpath tree
// when dir is "" (the root Skill), otherwise the rows under dir, excluding
// .git segments.
func skillRows(all []treeLine, dir string) []treeLine {
	var rows []treeLine
	for _, l := range all {
		if dir != "" {
			if !strings.HasPrefix(l.rel, dir+"/") {
				continue
			}
		}
		if isDotGitPath(l.rel) {
			continue
		}
		rows = append(rows, l)
	}
	return rows
}

// skillTreeDigest returns the canonical content digest of one Skill tree,
// with paths relative to the Skill root. Directory nodes are implicit and
// synthesized, so the digest matches an equivalent materialized Local
// Source tree.
func skillTreeDigest(rows []treeLine, dir string, blobs map[string][]byte) string {
	var nodes []canonNode
	seenDirs := map[string]bool{}
	addDirs := func(fileRel string) {
		dir := fileRel
		for {
			if i := strings.LastIndex(dir, "/"); i < 0 {
				dir = "."
			} else {
				dir = dir[:i]
			}
			if !seenDirs[dir] {
				seenDirs[dir] = true
				nodes = append(nodes, canonNode{relPath: dir, kind: canonKindDir})
			}
			if dir == "." {
				return
			}
		}
	}
	for _, l := range rows {
		rel := l.rel
		if dir != "" {
			rel = strings.TrimPrefix(rel, dir+"/")
		}
		addDirs(rel)
		n := canonNode{relPath: rel}
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
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		addDirs("SKILL.md") // an empty tree still has its root directory
	}
	return canonDigest(nodes)
}

// isNestedSkillDir reports whether dir lies inside another Skill directory.
func isNestedSkillDir(dir string, skills map[string]treeLine) bool {
	for ancestor := dir; ; {
		i := strings.LastIndex(ancestor, "/")
		if i < 0 {
			return false
		}
		ancestor = ancestor[:i]
		if _, ok := skills[ancestor]; ok {
			return true
		}
	}
}

func hasSkippedSegment(dir string) bool {
	for _, seg := range strings.Split(dir, "/") {
		if skippedDirs[seg] {
			return true
		}
	}
	return false
}

// addSkillBlob validates one SKILL.md marker and records the entry with its
// complete-tree digest. A marker that is not a regular file (symlink,
// gitlink, or other special mode) is reported as an invalid entry without
// being read.
func (obs *Observation) addSkillBlob(marker treeLine, relDir, digest string, data []byte, ok bool) {
	if marker.mode != "100644" && marker.mode != "100755" {
		obs.Issues = append(obs.Issues, Issue{RelativeDir: relDir, Reason: "SKILL.md is not a regular file"})
		return
	}
	if !ok {
		obs.Issues = append(obs.Issues, Issue{RelativeDir: relDir, Reason: "reading SKILL.md: blob is missing"})
		return
	}
	name, description, err := skillFrontmatter(data)
	if err != nil {
		obs.Issues = append(obs.Issues, Issue{RelativeDir: relDir, Reason: err.Error()})
		return
	}
	obs.Entries = append(obs.Entries, Entry{RelativeDir: relDir, Name: name, Description: description, Digest: digest})
}
