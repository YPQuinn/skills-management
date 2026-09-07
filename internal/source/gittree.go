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
	size int64 // byte size from `ls-tree -l`; -1 when unknown
}

type discoverMode int

const (
	discoverFull discoverMode = iota
	discoverListing
)

func (m discoverMode) String() string {
	if m == discoverListing {
		return "listing"
	}
	return "full"
}

// discoverCommit validates the Source subpath at the commit, lists the tree
// under it with NUL-safe plumbing, and applies the same depth-three
// discovery rules as Discover. Full mode fetches every Skill blob once and
// records complete-tree digests. Listing mode fetches only SKILL.md blobs.
func discoverCommit(ctx context.Context, cache, commit string, loc Locator, mode discoverMode) (Observation, error) {
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

	all := make([]treeLine, 0, len(rows))
	// relative Skill directory -> its SKILL.md listing row
	skills := map[string]treeLine{}
	for _, r := range rows {
		r.rel, err = gitRelativePath(r.path, loc.Subpath)
		if err != nil {
			return Observation{}, err
		}
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
		if err := obs.recordSkills(ctx, cache, all, map[string]treeLine{".": marker}, []string{""}, loc, mode); err != nil {
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
		if len(strings.Split(dir, "/")) > 3 {
			continue
		}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	if err := obs.recordSkills(ctx, cache, all, skills, dirs, loc, mode); err != nil {
		return Observation{}, err
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

// recordSkills validates Skill candidates from one shared blob fetch.
// Listing mode reads only SKILL.md markers. Full mode reads every blob in
// every candidate tree once, then computes each complete-tree digest.
func (obs *Observation) recordSkills(ctx context.Context, cache string, all []treeLine, skills map[string]treeLine, dirs []string, loc Locator, mode discoverMode) error {
	needed := make([]treeLine, 0, len(dirs))
	for _, dir := range dirs {
		key := dir
		if key == "" {
			key = "."
		}
		marker, ok := skills[key]
		if !ok {
			continue
		}
		if mode == discoverListing {
			needed = append(needed, marker)
			continue
		}
		needed = append(needed, skillRows(all, dir)...)
	}
	blobs, err := fetchTreeBlobs(ctx, cache, needed, loc)
	if err != nil {
		return fmt.Errorf("reading Skill tree: %v", err)
	}
	for _, dir := range dirs {
		relDir := dir
		if relDir == "" {
			relDir = "."
		}
		marker := skills[relDir]
		if mode == discoverListing {
			data, ok := blobs[marker.oid]
			obs.addSkillBlob(marker, relDir, "", data, ok)
			continue
		}
		rows := skillRows(all, dir)
		digest, err := skillTreeEffectiveDigest(rows, dir, blobs)
		if err != nil {
			obs.Issues = append(obs.Issues, Issue{RelativeDir: relDir, Reason: err.Error()})
			continue
		}
		data, ok := blobs[marker.oid]
		obs.addSkillBlob(marker, relDir, digest, data, ok)
	}
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

// gitSkillTreePath is the repository path passed to ls-tree for one Skill.
func gitSkillTreePath(subpath, relativeDir string) string {
	if relativeDir == "" || relativeDir == "." {
		return subpath
	}
	if subpath == "" {
		return relativeDir
	}
	return subpath + "/" + relativeDir
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
