package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGitObserveListingOmitsTreeDigest(t *testing.T) {
	bare, _ := makeRemote(t, map[string]string{
		"skills/alpha/SKILL.md": "---\nname: Alpha\ndescription: one\n---\n",
		"skills/alpha/notes.md": "keep\n",
	}, "main")
	workDir := t.TempDir()
	loc := gitLoc(t, bare, "", "")
	var g Git
	listed, err := g.ObserveListing(context.Background(), loc, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "Alpha" {
		t.Fatalf("listing entries: %+v", listed.Entries)
	}
	if listed.Entries[0].Digest != "" {
		t.Fatalf("listing digest must be empty, got %q", listed.Entries[0].Digest)
	}
	full, err := g.Observe(context.Background(), loc, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if full.Entries[0].Digest == "" {
		t.Fatal("full observation must record a complete-tree digest")
	}
}

func TestMaterializeDoesNotFetchWhenCommitCached(t *testing.T) {
	gitAvailable(t)
	bare, commit := makeRemote(t, map[string]string{
		"skills/alpha/SKILL.md": "---\nname: Alpha\ndescription: one\n---\n",
	}, "main")
	workDir := t.TempDir()
	loc := gitLoc(t, bare, "", "")
	var g Git
	obs, err := g.Observe(context.Background(), loc, workDir)
	if err != nil {
		t.Fatal(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "git.log")
	wrapper := filepath.Join(t.TempDir(), "git")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + strconv.Quote(logPath) + "\nexec " + strconv.Quote(gitPath) + " \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SKILLCTL_GIT", wrapper)
	dst := t.TempDir()
	if _, err := MaterializeEntry(context.Background(), loc, commit, obs.Entries[0], workDir, dst, false); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(logged), "\n") {
		fields := strings.Fields(line)
		for _, f := range fields {
			if f == "fetch" {
				t.Fatalf("materialize fetched the remote:\n%s", logged)
			}
		}
	}
}
