package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const lfsPointerContent = "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize 12345\n"

// gitRun executes git in dir, failing the test on error.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// observeGitEntry observes a Git Source and returns the entry for the
// relative dir plus the observed commit.
func observeGitEntry(t *testing.T, loc Locator, workDir, relativeDir string) (Entry, string) {
	t.Helper()
	var g Git
	obs, err := g.Observe(context.Background(), loc, workDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range obs.Entries {
		if e.RelativeDir == relativeDir {
			return e, obs.Commit
		}
	}
	t.Fatalf("entry %q not found in %+v", relativeDir, obs.Entries)
	return Entry{}, ""
}

// observeGitIssues observes a Git Source and returns its recorded Issues.
func observeGitIssues(t *testing.T, loc Locator, workDir string) []Issue {
	t.Helper()
	var g Git
	obs, err := g.Observe(context.Background(), loc, workDir)
	if err != nil {
		t.Fatal(err)
	}
	return obs.Issues
}

// materializeGitEntry runs MaterializeEntry for a Git locator.
func materializeGitEntry(t *testing.T, loc Locator, commit string, entry Entry, workDir, dst string) string {
	t.Helper()
	digest, err := MaterializeEntry(context.Background(), loc, commit, entry, workDir, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestMaterializeGitCopiesCompleteTree(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md":     {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/tools/run.sh": {content: "#!/bin/sh\necho hi\n", mode: 0o755},
		"skills/alpha/refs/x.md":    {content: "deep", mode: 0o644},
		"skills/alpha/empty/keep":   {content: "not empty in git", mode: 0o644},
	}
	bare, _ := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	entry, commit := observeGitEntry(t, loc, workDir, "skills/alpha")
	dst := t.TempDir()
	digest := materializeGitEntry(t, loc, commit, entry, workDir, dst)

	kinds := walkKinds(t, dst)
	for _, want := range []string{".", "SKILL.md", "tools", "tools/run.sh", "refs", "refs/x.md", "empty", "empty/keep"} {
		if kinds[want] != "file" && kinds[want] != "dir" {
			t.Fatalf("materialized tree missing %q: %v", want, kinds)
		}
	}
	info, err := os.Lstat(filepath.Join(dst, "tools", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("executable bit must be preserved")
	}
	if got, err := TreeDigest(context.Background(), dst); err != nil || got != digest {
		t.Fatalf("returned digest %s does not match the tree digest %s (%v)", digest, got, err)
	}
	// a symlink-free tree must produce exactly the observed digest
	if digest != entry.Digest {
		t.Fatalf("materialized digest %s must equal the observation digest %s for a symlink-free tree", digest, entry.Digest)
	}
}

func TestMaterializeGitDereferencesInternalSymlinks(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md":     {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/tools/run.sh": {content: "#!/bin/sh\n", mode: 0o755},
		"skills/alpha/link-file":    {symlink: "tools/run.sh"},
		"skills/alpha/link-dir":     {symlink: "tools"},
	}
	bare, _ := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	entry, commit := observeGitEntry(t, loc, workDir, "skills/alpha")
	dst := t.TempDir()
	digest := materializeGitEntry(t, loc, commit, entry, workDir, dst)

	kinds := walkKinds(t, dst)
	if kinds["link-file"] != "file" || kinds["link-dir"] != "dir" || kinds["link-dir/run.sh"] != "file" {
		t.Fatalf("symlinks not dereferenced: %v", kinds)
	}
	for p, k := range kinds {
		if k == "symlink" {
			t.Fatalf("symlink %q survived materialization", p)
		}
	}
	data, err := os.ReadFile(filepath.Join(dst, "link-file"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "#!/bin/sh\n" {
		t.Fatalf("dereferenced content: %q", data)
	}
	// the observation digest is the effective materialized identity
	if digest != entry.Digest {
		t.Fatalf("materialized digest %s must equal the observation digest %s for a dereferenced symlink tree", digest, entry.Digest)
	}
}
