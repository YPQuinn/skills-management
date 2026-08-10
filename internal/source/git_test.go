package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/lock"
)

// gitAvailable reports whether the real git executable is usable; Git tests
// are hermetic (local bare remotes via file:// URLs) but still need git.
func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

// makeRemote builds a working repository with one commit and returns its
// bare clone path plus the initial commit SHA.
func makeRemote(t *testing.T, files map[string]string, branch string) (bare string, commit string) {
	t.Helper()
	gitAvailable(t)
	work := t.TempDir()
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
	for path, content := range files {
		full := filepath.Join(work, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(work, "add", ".")
	run(work, "commit", "-m", "initial")
	commit = strings.TrimSpace(run(work, "rev-parse", "HEAD"))

	bare = filepath.Join(t.TempDir(), "remote.git")
	run(t.TempDir(), "clone", "--bare", work, bare)
	return bare, commit
}

func gitLoc(t *testing.T, bare, ref, subpath string) Locator {
	t.Helper()
	loc, err := Normalize(KindGit, "file://"+bare, ref, subpath)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestGitObserveDefaultBranch(t *testing.T) {
	bare, commit := makeRemote(t, map[string]string{
		"skills/alpha/SKILL.md":     "---\nname: Alpha\ndescription: one\n---\n",
		"skills/beta/deep/SKILL.md": "---\nname: Deep\ndescription: two\n---\n",
		"node_modules/x/SKILL.md":   "---\nname: Hidden\ndescription: skip\n---\n",
	}, "main")

	var g Git
	obs, err := g.Observe(context.Background(), gitLoc(t, bare, "", ""), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if obs.Commit != commit {
		t.Fatalf("commit: got %q, want %q", obs.Commit, commit)
	}
	var dirs []string
	for _, e := range obs.Entries {
		dirs = append(dirs, e.RelativeDir)
	}
	if strings.Join(dirs, ",") != "skills/alpha,skills/beta/deep" {
		t.Fatalf("entries: got %v", dirs)
	}
}

func TestGitObserveAnnotatedTagRecordsPeeledCommit(t *testing.T) {
	bare, commit := makeRemote(t, map[string]string{
		"SKILL.md": "---\nname: Alpha\ndescription: one\n---\n",
	}, "main")
	cmd := exec.Command("git", "-c", "user.name=Test", "-c", "user.email=test@example.com",
		"--git-dir", bare, "tag", "-a", "v1", "-m", "v1", commit)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create annotated tag: %v\n%s", err, out)
	}

	obs, err := (Git{}).Observe(context.Background(), gitLoc(t, bare, "v1", ""), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if obs.Commit != commit {
		t.Fatalf("commit: got %q, want peeled %q", obs.Commit, commit)
	}
}

func TestGitObserveSubpathAndRootSkill(t *testing.T) {
	bare, commit := makeRemote(t, map[string]string{
		"repo/SKILL.md":          "---\nname: Root\ndescription: at root\n---\n",
		"repo/skills/x/SKILL.md": "---\nname: X\ndescription: nested\n---\n",
	}, "main")

	var g Git
	// subpath whose root directly contains SKILL.md short-circuits
	obs, err := g.Observe(context.Background(), gitLoc(t, bare, "", "repo"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if obs.Commit != commit || len(obs.Entries) != 1 || obs.Entries[0].RelativeDir != "." {
		t.Fatalf("root skill: %+v", obs)
	}

	// subpath traversal is rejected at normalization
	if _, err := Normalize(KindGit, "file://"+bare, "", "../x"); err == nil {
		t.Fatal("traversing subpath: want error")
	}
}

func TestGitObserveUnavailableAndInvalid(t *testing.T) {
	var g Git
	loc, err := Normalize(KindGit, "file://"+filepath.Join(t.TempDir(), "missing.git"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Observe(context.Background(), loc, t.TempDir()); err == nil {
		t.Fatal("missing repository: want error")
	}

	// invalid frontmatter surfaces as an Issue, not an error
	bare, _ := makeRemote(t, map[string]string{
		"skills/bad/SKILL.md": "# no frontmatter\n",
		"skills/ok/SKILL.md":  "---\nname: Ok\ndescription: fine\n---\n",
	}, "main")
	obs, err := g.Observe(context.Background(), gitLoc(t, bare, "", ""), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 1 || obs.Entries[0].Name != "Ok" {
		t.Fatalf("entries: %+v", obs.Entries)
	}
	if len(obs.Issues) != 1 || obs.Issues[0].RelativeDir != "skills/bad" {
		t.Fatalf("issues: %+v", obs.Issues)
	}
}

func TestGitCacheLock(t *testing.T) {
	gitAvailable(t)
	bare, _ := makeRemote(t, map[string]string{
		"SKILL.md": "---\nname: Alpha\ndescription: one\n---\n",
	}, "main")
	workDir := t.TempDir()
	loc := gitLoc(t, bare, "", "")

	cache := gitCacheDir(workDir, loc.Location)
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(cache + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Unlock()
	var g Git
	if _, err := g.Observe(context.Background(), loc, workDir); err == nil {
		t.Fatal("observe under a held cache lock: want error")
	}
}
