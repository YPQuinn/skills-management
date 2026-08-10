package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitObserveDigests(t *testing.T) {
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
	writeGit := func(path, content string) {
		t.Helper()
		full := filepath.Join(work, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(work, "init", "-b", "main")
	run(work, "config", "user.email", "test@example.com")
	run(work, "config", "user.name", "Test")
	writeGit("skills/alpha/SKILL.md", "---\nname: Alpha\ndescription: one\n---\n")
	writeGit("skills/alpha/references/deep/a.md", "supporting")
	writeGit("skills/beta/SKILL.md", "---\nname: Beta\ndescription: two\n---\n")
	run(work, "add", ".")
	run(work, "commit", "-m", "one")
	bare := filepath.Join(t.TempDir(), "remote.git")
	run(t.TempDir(), "clone", "--bare", work, bare)

	workDir := t.TempDir()
	var g Git
	loc := gitLoc(t, bare, "", "")
	first, err := g.Observe(context.Background(), loc, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 2 || first.Digest == "" {
		t.Fatalf("first observation: %+v", first)
	}
	digests := map[string]string{}
	for _, e := range first.Entries {
		if len(e.Digest) != 64 {
			t.Fatalf("entry digest must be a full SHA-256: %q", e.Digest)
		}
		digests[e.RelativeDir] = e.Digest
	}

	// digests are deterministic across observations
	second, err := g.Observe(context.Background(), loc, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if second.Digest != first.Digest {
		t.Fatalf("inventory digest must be deterministic: %q vs %q", second.Digest, first.Digest)
	}
	for _, e := range second.Entries {
		if e.Digest != digests[e.RelativeDir] {
			t.Fatalf("entry digest must be deterministic: %q vs %q", e.Digest, digests[e.RelativeDir])
		}
	}

	// a supporting-file change inside alpha rebinds alpha's digest and the
	// aggregate, leaving beta's digest untouched
	writeGit("skills/alpha/references/deep/a.md", "changed supporting")
	run(work, "add", ".")
	run(work, "commit", "-m", "two")
	run(work, "push", bare, "main")
	third, err := g.Observe(context.Background(), loc, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if third.Digest == first.Digest {
		t.Fatal("a supporting-file change must change the inventory digest")
	}
	for _, e := range third.Entries {
		if e.RelativeDir == "skills/alpha" && e.Digest == digests["skills/alpha"] {
			t.Fatal("a supporting-file change must change the alpha entry digest")
		}
		if e.RelativeDir == "skills/beta" && e.Digest != digests["skills/beta"] {
			t.Fatal("an unrelated Skill digest must stay stable")
		}
	}
}

// TestGitEntryDigestLocationAndSubpathIndependent proves a Skill tree's
// digest is a content identity: the same tree produces the same digest when
// registered at the repository root or under a configured subpath, and when
// it lives in different repositories entirely.
func TestGitEntryDigestLocationAndSubpathIndependent(t *testing.T) {
	gitAvailable(t)
	content := map[string]string{
		"skills/alpha/SKILL.md":  "---\nname: Alpha\ndescription: one\n---\n",
		"skills/alpha/refs/a.md": "supporting",
	}
	// two repositories with identical Skill content: one registered at the
	// root, one registered under the "repo" subpath
	rootBare, _ := makeRemote(t, content, "main")

	subContent := map[string]string{}
	for path, c := range content {
		subContent["repo/"+path] = c
	}
	subBare, _ := makeRemote(t, subContent, "main")

	workDir := t.TempDir()
	var g Git
	rootObs, err := g.Observe(context.Background(), gitLoc(t, rootBare, "", ""), workDir)
	if err != nil {
		t.Fatal(err)
	}
	subObs, err := g.Observe(context.Background(), gitLoc(t, subBare, "", "repo"), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rootObs.Entries) != 1 || len(subObs.Entries) != 1 {
		t.Fatalf("observations: %+v / %+v", rootObs.Entries, subObs.Entries)
	}
	if rootObs.Entries[0].RelativeDir != "skills/alpha" || subObs.Entries[0].RelativeDir != "skills/alpha" {
		t.Fatalf("entries: %q / %q", rootObs.Entries[0].RelativeDir, subObs.Entries[0].RelativeDir)
	}
	if rootObs.Entries[0].Digest != subObs.Entries[0].Digest {
		t.Fatalf("entry digest must be independent of location and subpath: %q vs %q",
			rootObs.Entries[0].Digest, subObs.Entries[0].Digest)
	}
	if rootObs.Digest != subObs.Digest {
		t.Fatalf("aggregate digest must be independent of location and subpath: %q vs %q",
			rootObs.Digest, subObs.Digest)
	}

	// the root-Skill shape is subpath independent too: SKILL.md directly in
	// the subpath root hashes the same as SKILL.md at the repository root
	rootSkillBare, _ := makeRemote(t, map[string]string{
		"SKILL.md":  "---\nname: Root\ndescription: root skill\n---\n",
		"refs/a.md": "supporting",
	}, "main")
	subSkillBare, _ := makeRemote(t, map[string]string{
		"repo/SKILL.md":  "---\nname: Root\ndescription: root skill\n---\n",
		"repo/refs/a.md": "supporting",
	}, "main")
	rootSkill, err := g.Observe(context.Background(), gitLoc(t, rootSkillBare, "", ""), workDir)
	if err != nil {
		t.Fatal(err)
	}
	subSkill, err := g.Observe(context.Background(), gitLoc(t, subSkillBare, "", "repo"), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rootSkill.Entries) != 1 || len(subSkill.Entries) != 1 {
		t.Fatalf("root skills: %+v / %+v", rootSkill.Entries, subSkill.Entries)
	}
	if rootSkill.Entries[0].Digest != subSkill.Entries[0].Digest {
		t.Fatalf("root-Skill digest must be subpath independent: %q vs %q",
			rootSkill.Entries[0].Digest, subSkill.Entries[0].Digest)
	}
}
