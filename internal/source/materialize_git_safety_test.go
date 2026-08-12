package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObservationRejectsGitEscapingSymlink(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/leak":     {symlink: "../outside.txt"},
	}
	bare, _ := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	issues := observeGitIssues(t, loc, workDir)
	assertIssue(t, issues, "skills/alpha", "escapes the Skill root")
}

func TestMaterializeGitRejectsEscapingSymlink(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/leak":     {symlink: "../outside.txt"},
	}
	bare, work := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()
	commit := strings.TrimSpace(gitRun(t, work, "rev-parse", "HEAD"))

	_, err := MaterializeEntry(context.Background(), loc, commit, Entry{RelativeDir: "skills/alpha", Digest: "stale"}, workDir, t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "escapes the Skill root") {
		t.Fatalf("escaping symlink: got %v, want an escape error", err)
	}
}

func TestObservationRejectsGitBrokenSymlink(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/dangling": {symlink: "missing.txt"},
	}
	bare, _ := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	issues := observeGitIssues(t, loc, workDir)
	assertIssue(t, issues, "skills/alpha", "missing")
}

func TestMaterializeGitRejectsBrokenSymlink(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/dangling": {symlink: "missing.txt"},
	}
	bare, work := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()
	commit := strings.TrimSpace(gitRun(t, work, "rev-parse", "HEAD"))

	_, err := MaterializeEntry(context.Background(), loc, commit, Entry{RelativeDir: "skills/alpha", Digest: "stale"}, workDir, t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("broken symlink: got %v, want a missing-target error", err)
	}
}

func TestObservationRejectsGitGitlink(t *testing.T) {
	gitAvailable(t)
	work := t.TempDir()
	gitRun(t, work, "init", "-b", "main")
	gitRun(t, work, "config", "user.email", "test@example.com")
	gitRun(t, work, "config", "user.name", "Test")
	full := filepath.Join(work, "skills", "alpha", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("---\nname: Alpha\ndescription: one\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-m", "initial")
	head := strings.TrimSpace(gitRun(t, work, "rev-parse", "HEAD"))
	// a gitlink (mode 160000) is added through plumbing, as a submodule
	// checkout would produce
	gitRun(t, work, "update-index", "--add", "--cacheinfo", "160000,"+head+",skills/alpha/submodule")
	gitRun(t, work, "commit", "-m", "add submodule")

	bare := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, t.TempDir(), "clone", "--bare", work, bare)
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	issues := observeGitIssues(t, loc, workDir)
	assertIssue(t, issues, "skills/alpha", "gitlink")
}

func TestMaterializeGitRejectsGitlink(t *testing.T) {
	gitAvailable(t)
	work := t.TempDir()
	gitRun(t, work, "init", "-b", "main")
	gitRun(t, work, "config", "user.email", "test@example.com")
	gitRun(t, work, "config", "user.name", "Test")
	full := filepath.Join(work, "skills", "alpha", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("---\nname: Alpha\ndescription: one\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-m", "initial")
	head := strings.TrimSpace(gitRun(t, work, "rev-parse", "HEAD"))
	gitRun(t, work, "update-index", "--add", "--cacheinfo", "160000,"+head+",skills/alpha/submodule")
	gitRun(t, work, "commit", "-m", "add submodule")

	bare := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, t.TempDir(), "clone", "--bare", work, bare)
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()
	commit := strings.TrimSpace(gitRun(t, work, "rev-parse", "HEAD"))

	_, err := MaterializeEntry(context.Background(), loc, commit, Entry{RelativeDir: "skills/alpha", Digest: "stale"}, workDir, t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "gitlink") {
		t.Fatalf("gitlink: got %v, want a gitlink error", err)
	}
}

func TestMaterializeGitRejectsLFSPointer(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/data.bin": {content: lfsPointerContent, mode: 0o644},
	}
	bare, _ := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	entry, commit := observeGitEntry(t, loc, workDir, "skills/alpha")
	_, err := MaterializeEntry(context.Background(), loc, commit, entry, workDir, t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "LFS pointer") {
		t.Fatalf("LFS pointer: got %v, want an LFS error", err)
	}
}

func TestMaterializeGitRejectsWrongCommit(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
	}
	bare, work := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	entry, _ := observeGitEntry(t, loc, workDir, "skills/alpha")

	// the Source advances after the observation
	if err := os.WriteFile(filepath.Join(work, "skills", "alpha", "new.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-m", "advance")
	gitRun(t, work, "push", bare, "main")
	newCommit := strings.TrimSpace(gitRun(t, work, "rev-parse", "HEAD"))

	// materializing the newer commit against the older observation's digest
	// must reject instead of installing different content
	_, err := MaterializeEntry(context.Background(), loc, newCommit, entry, workDir, t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("wrong commit: got %v, want a digest-mismatch error", err)
	}
}

func TestMaterializeGitRequiresFullCommitSHA(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
	}
	bare, _ := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	entry, _ := observeGitEntry(t, loc, workDir, "skills/alpha")
	_, err := MaterializeEntry(context.Background(), loc, "main", entry, workDir, t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "full observed commit") {
		t.Fatalf("short commit: got %v, want a full-SHA error", err)
	}
}
