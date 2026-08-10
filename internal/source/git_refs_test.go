package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitObserveRefsAndUpdates(t *testing.T) {
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
		return strings.TrimSpace(string(out))
	}
	run(work, "init", "-b", "main")
	run(work, "config", "user.email", "test@example.com")
	run(work, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work, "SKILL.md"),
		[]byte("---\nname: Alpha\ndescription: one\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(work, "add", ".")
	run(work, "commit", "-m", "one")
	first := run(work, "rev-parse", "HEAD")

	// a branch and a tag both point at the first commit
	run(work, "branch", "stable")
	run(work, "tag", "v1")
	bare := filepath.Join(t.TempDir(), "remote.git")
	run(t.TempDir(), "clone", "--bare", work, bare)

	workDir := t.TempDir()
	var g Git
	for _, ref := range []string{"", "stable", "v1", "refs/heads/stable"} {
		obs, err := g.Observe(context.Background(), gitLoc(t, bare, ref, ""), workDir)
		if err != nil {
			t.Fatalf("ref %q: %v", ref, err)
		}
		if obs.Commit != first {
			t.Fatalf("ref %q: commit %q, want %q", ref, obs.Commit, first)
		}
	}
	// a pinned full SHA stays pinned
	pinned, err := g.Observe(context.Background(), gitLoc(t, bare, first, ""), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if pinned.Commit != first {
		t.Fatalf("pinned commit: got %q", pinned.Commit)
	}

	// a new commit on the default branch moves the observed commit; the
	// remote advances and a fresh check on the same cache sees the update
	run(work, "commit", "--allow-empty", "-m", "two")
	second := run(work, "rev-parse", "HEAD")
	run(work, "push", bare, "main")
	obs, err := g.Observe(context.Background(), gitLoc(t, bare, "", ""), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Commit != second {
		t.Fatalf("updated commit: got %q, want %q", obs.Commit, second)
	}

	// an unknown ref fails; a pinned SHA that is not reachable fails
	if _, err := g.Observe(context.Background(), gitLoc(t, bare, "nope", ""), workDir); err == nil {
		t.Fatal("unknown ref: want error")
	}
	if _, err := g.Observe(context.Background(), gitLoc(t, bare, strings.Repeat("0", 40), ""), workDir); err == nil {
		t.Fatal("unreachable pinned SHA: want error")
	}
}

func TestGitObserveSubpathValidation(t *testing.T) {
	bare, _ := makeRemote(t, map[string]string{
		"skills/alpha/SKILL.md": "---\nname: Alpha\ndescription: one\n---\n",
		"docs/readme.md":        "docs",
		"notes.txt":             "hello",
	}, "main")
	workDir := t.TempDir()
	var g Git

	// an existing tree subpath is valid and yields its Skills
	obs, err := g.Observe(context.Background(), gitLoc(t, bare, "", "skills"), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 1 || obs.Entries[0].RelativeDir != "alpha" {
		t.Fatalf("subpath skills: %+v", obs.Entries)
	}

	// a genuinely existing tree with zero Skills is valid, not an error
	obs, err = g.Observe(context.Background(), gitLoc(t, bare, "", "docs"), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 0 || len(obs.Issues) != 0 {
		t.Fatalf("empty tree subpath: %+v", obs)
	}

	// a missing subpath fails instead of appearing as an empty Inventory
	if _, err := g.Observe(context.Background(), gitLoc(t, bare, "", "nope"), workDir); err == nil {
		t.Fatal("missing subpath: want error")
	}
	// a file subpath fails too
	if _, err := g.Observe(context.Background(), gitLoc(t, bare, "", "notes.txt"), workDir); err == nil {
		t.Fatal("file subpath: want error")
	}
}
