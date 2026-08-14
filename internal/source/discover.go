package source

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skippedDirs are never searched for Skills: VCS metadata, dependency
// installs, build output, and caches.
var skippedDirs = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"out":          true,
	"target":       true,
	"__pycache__":  true,
	".cache":       true,
}

// Discover scans a materialized local tree for Skills: a SKILL.md directly
// in root is the whole Inventory and discovery stops; otherwise SKILL.md is
// searched up to three directory levels below root, without descending into
// a directory that already contains one. Entries and issues are returned in
// deterministic relative-directory order.
func Discover(root string) (Observation, error) {
	return DiscoverCtx(context.Background(), root)
}

// DiscoverCtx is Discover with prompt context-cancellation support.
func DiscoverCtx(ctx context.Context, root string) (Observation, error) {
	dirs, err := skillCandidates(ctx, root)
	if err != nil {
		return Observation{}, err
	}
	var obs Observation
	for _, dir := range dirs {
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return Observation{}, err
		}
		obs.addSkillDir(dir, filepath.ToSlash(rel))
	}
	sortObservation(&obs)
	return obs, nil
}

// markerIsRegular reports whether the SKILL.md at path is a regular file,
// without following symlinks or opening the node. Discovery never opens a
// non-regular marker, so a FIFO, socket, device, directory, or symlink can
// never block a scan or be read as ordinary content.
func markerIsRegular(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// skillCandidates returns the absolute directories that contain a SKILL.md
// under root, following the discovery contract: a SKILL.md directly in root
// makes root the only candidate; otherwise directories containing SKILL.md
// are found up to three levels below root, skipped directories are never
// entered, and a candidate is not descended into. Existence is detected with
// Lstat so a non-regular marker still makes its directory a candidate; the
// marker is then reported as an invalid entry without ever being opened.
func skillCandidates(ctx context.Context, root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	if _, err := os.Lstat(filepath.Join(root, "SKILL.md")); err == nil {
		return []string{root}, nil
	}

	var dirs []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return err
		}
		if path == root || !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if skippedDirs[d.Name()] || len(strings.Split(rel, string(filepath.Separator))) > 3 {
			return filepath.SkipDir
		}
		if _, err := os.Lstat(filepath.Join(path, "SKILL.md")); err == nil {
			dirs = append(dirs, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

// addSkillDir validates one Skill directory and records it as an Entry or an
// Issue, always under its relative directory.
func (obs *Observation) addSkillDir(path, rel string) {
	name, description, err := readSkill(path)
	if err != nil {
		obs.Issues = append(obs.Issues, Issue{RelativeDir: filepath.ToSlash(rel), Reason: err.Error()})
		return
	}
	obs.Entries = append(obs.Entries, Entry{RelativeDir: filepath.ToSlash(rel), Name: name, Description: description})
}

func readSkill(dir string) (string, string, error) {
	marker := filepath.Join(dir, "SKILL.md")
	if !markerIsRegular(marker) {
		return "", "", fmt.Errorf("SKILL.md is not a regular file")
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		return "", "", fmt.Errorf("reading SKILL.md: %v", err)
	}
	return skillFrontmatter(data)
}

// ValidateSkillDir reports whether the directory at dir carries a legal
// Skill: a regular SKILL.md marker whose frontmatter names a Skill. It is
// the shared Skill validator for Source observation and Store evaluation,
// so a Store with a missing, non-regular, or invalid marker classifies as
// store_invalid exactly like an invalid Source entry.
func ValidateSkillDir(dir string) error {
	_, _, err := readSkill(dir)
	return err
}

func sortObservation(obs *Observation) {
	sort.Slice(obs.Entries, func(i, j int) bool { return obs.Entries[i].RelativeDir < obs.Entries[j].RelativeDir })
	sort.Slice(obs.Issues, func(i, j int) bool { return obs.Issues[i].RelativeDir < obs.Issues[j].RelativeDir })
}
