package app

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/source"
)

func TestGitAddListsThenImportObservesOnceThenReuses(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	a := newTestApp(t)
	work := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	writeSourceSkill(t, work, "alpha")
	run("add", ".")
	run("commit", "-m", "initial")
	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "clone", "--bare", work, bare).CombinedOutput(); err != nil {
		t.Fatalf("bare clone: %v\n%s", err, out)
	}

	src, err := a.AddSource(context.Background(), source.AddInput{
		Kind: source.KindGit, Location: "file://" + bare, Name: "speed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Entries) != 1 || src.Entries[0].Digest != "" {
		t.Fatalf("add must persist a listing Inventory: %+v", src.Entries)
	}

	n := 0
	a.observer = countingObserver{Observer: a.observer, n: &n}
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Imported != 1 {
		t.Fatalf("import: %+v", res)
	}
	if n != 1 {
		t.Fatalf("import must take one full observation, got %d", n)
	}
	shown, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Entries[0].Digest == "" {
		t.Fatal("import must fill complete-tree digests")
	}

	before := n
	res, err = a.ImportSkills(context.Background(), ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.AlreadyImported != 1 {
		t.Fatalf("second import: %+v", res)
	}
	if n != before {
		t.Fatalf("same-commit import must reuse Inventory, Observe called %d extra times", n-before)
	}
}

func TestReuseGitObservationRequiresCompleteDigests(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	cur := &source.Source{
		Locator:    source.Locator{Kind: source.KindGit},
		Available:  true,
		LastCommit: strings.Repeat("a", 40),
		Entries:    []source.Entry{{RelativeDir: "skills/alpha", Name: "Alpha"}},
	}
	reused, err := a.reuseGitObservation(context.Background(), cur)
	if err != nil {
		t.Fatal(err)
	}
	if reused != nil {
		t.Fatal("listing-only Inventory must not be reused")
	}
}
