package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// treeFile is one file of a test repository: either a regular file with
// content and mode, or a symlink with the given target.
type treeFile struct {
	content string
	mode    os.FileMode
	symlink string
}

// buildRepo creates a working repository plus its bare clone and returns
// both paths plus the initial commit. The work tree doubles as the Local
// Source mirror of the same content.
func buildRepo(t *testing.T, files map[string]treeFile, branch string) (bare, work string) {
	t.Helper()
	gitAvailable(t)
	work = t.TempDir()
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run(work, "init", "-b", branch)
	run(work, "config", "user.email", "test@example.com")
	run(work, "config", "user.name", "Test")
	for path, f := range files {
		full := filepath.Join(work, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if f.symlink != "" {
			if err := os.Symlink(f.symlink, full); err != nil {
				t.Fatalf("symlink %s: %v", path, err)
			}
			continue
		}
		if err := os.WriteFile(full, []byte(f.content), f.mode); err != nil {
			t.Fatal(err)
		}
	}
	run(work, "add", ".")
	run(work, "commit", "-m", "initial")
	bare = filepath.Join(t.TempDir(), "remote.git")
	run(t.TempDir(), "clone", "--bare", work, bare)
	// the Local mirror must be the fully resolved path a registered Source
	// stores, so the root-identity check passes on every platform.
	resolved, err := filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	return bare, resolved
}

// assertSameDigests observes one equivalent tree through the Git and Local
// observers and requires identical entries, entry digests, and aggregate
// Inventory digests. The trees include an executable file and a symlink,
// and file names with spaces, tabs, newlines, and Unicode. localRoot is the
// Local scan directory (the nested catalog path when Git uses a subpath).
func assertSameDigests(t *testing.T, bare, localRoot, gitSubpath string) {
	t.Helper()
	var g Git
	gitObs, err := g.Observe(context.Background(), gitLoc(t, bare, "", gitSubpath), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	localObs, err := (Local{stabilityInterval: time.Millisecond}).Observe(
		context.Background(), Locator{Kind: KindLocal, Location: localRoot}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(gitObs.Entries) != len(localObs.Entries) {
		t.Fatalf("entry counts differ: git %+v vs local %+v", gitObs.Entries, localObs.Entries)
	}
	for i := range gitObs.Entries {
		g, l := gitObs.Entries[i], localObs.Entries[i]
		if g.RelativeDir != l.RelativeDir || g.Name != l.Name {
			t.Fatalf("entry identity differs: git %+v vs local %+v", g, l)
		}
		if g.Digest != l.Digest {
			t.Fatalf("entry digest differs for %q:\n  git   %s\n  local %s", g.RelativeDir, g.Digest, l.Digest)
		}
	}
	if gitObs.Digest != localObs.Digest {
		t.Fatalf("inventory digest differs:\n  git   %s\n  local %s", gitObs.Digest, localObs.Digest)
	}
}

// TestLocalAndGitRootSkillDigestsMatch is the regression for decision 05:
// the same materialized Skill tree must produce the same Entry and
// Inventory digests from a Local Source and a Git Source, including
// executable files, symlinks, and unusual file names.
func TestLocalAndGitRootSkillDigestsMatch(t *testing.T) {
	files := map[string]treeFile{
		"SKILL.md":           {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"run.sh":             {content: "#!/bin/sh\necho hi\n", mode: 0o755},
		"refs/with space.md": {content: "space", mode: 0o644},
		"refs/tab\tname.md":  {content: "tab", mode: 0o644},
		"refs/new\nline.md":  {content: "newline", mode: 0o644},
		"refs/ünïcode.md":    {content: "unicode", mode: 0o644},
		"link.md":            {symlink: "run.sh"},
		"refs/deep/nested/x": {content: "deep", mode: 0o644},
	}
	bare, work := buildRepo(t, files, "main")
	assertSameDigests(t, bare, work, "")
}

// TestLocalAndGitNestedSkillDigestsMatch covers the nested catalog shape:
// the same Skill observed through a Git subpath and a Local Source whose
// location is that subdirectory must yield identical digests, with entry
// paths relative to the scan root.
func TestLocalAndGitNestedSkillDigestsMatch(t *testing.T) {
	files := map[string]treeFile{
		"catalog/skills/alpha/SKILL.md":     {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"catalog/skills/alpha/tools/run.sh": {content: "#!/bin/sh\necho hi\n", mode: 0o755},
		"catalog/skills/alpha/link.md":      {symlink: "tools/run.sh"},
		"catalog/other/not-a-skill.txt":     {content: "ignore", mode: 0o644},
	}
	bare, work := buildRepo(t, files, "main")
	assertSameDigests(t, bare, filepath.Join(work, "catalog", "skills"), "catalog/skills")
}

// TestGitUnusualPathsParsedByPlumbing proves the NUL-safe ls-tree parsing
// survives paths containing spaces, tabs, newlines, and Unicode without
// relying on the repository's quoting configuration.
func TestGitUnusualPathsParsedByPlumbing(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md":      {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/with space.md": {content: "space", mode: 0o644},
		"skills/alpha/tab\tname.md":  {content: "tab", mode: 0o644},
		"skills/alpha/new\nline.md":  {content: "newline", mode: 0o644},
		"skills/alpha/ünïcode.md":    {content: "unicode", mode: 0o644},
	}
	bare, _ := buildRepo(t, files, "main")
	var g Git
	obs, err := g.Observe(context.Background(), gitLoc(t, bare, "", ""), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 1 || obs.Entries[0].RelativeDir != "skills/alpha" {
		t.Fatalf("entries: %+v", obs.Entries)
	}
	if obs.Entries[0].Digest == "" {
		t.Fatal("digest missing")
	}
	if len(obs.Issues) != 0 {
		t.Fatalf("issues: %+v", obs.Issues)
	}
	// the same tree materialized locally carries the same digest
	work := t.TempDir()
	for path, f := range files {
		full := filepath.Join(work, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), f.mode); err != nil {
			t.Fatal(err)
		}
	}
	realWork, err := filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	localObs, err := (Local{stabilityInterval: time.Millisecond}).Observe(
		context.Background(), Locator{Kind: KindLocal, Location: realWork}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(localObs.Entries) != 1 || localObs.Entries[0].Digest != obs.Entries[0].Digest {
		t.Fatalf("digest mismatch: git %q vs local %+v", obs.Entries[0].Digest, localObs.Entries)
	}
}

// TestParseTreeRecordsRoundTrip pins the -z parser on paths that would
// break line/tab splitting, independent of any git installation.
func TestParseTreeRecordsRoundTrip(t *testing.T) {
	paths := []string{
		"SKILL.md",
		"refs/with space.md",
		"refs/tab\tname.md",
		"refs/new\nline.md",
		"refs/ünïcode/emoji-😀.md",
	}
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString("100644 blob 0000000000000000000000000000000000000000\t")
		sb.WriteString(p)
		sb.WriteByte(0)
	}
	rows, err := parseTreeRecords(sb.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(paths) {
		t.Fatalf("rows: got %d, want %d", len(rows), len(paths))
	}
	for i, p := range paths {
		if rows[i].path != p || rows[i].mode != "100644" {
			t.Fatalf("row %d: %+v, want path %q", i, rows[i], p)
		}
	}
}
