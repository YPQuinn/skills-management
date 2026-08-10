package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const validSkill = "---\nname: %s\ndescription: %s\n---\n# Body\n"

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(validSkill, name, description)
	if name == "" && description == "" {
		content = "# no frontmatter\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverRootSkillShortCircuits(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "Alpha", "the alpha skill")
	// a nested skill that must never be reached
	writeSkill(t, filepath.Join(root, "skills", "hidden"), "Hidden", "nested")

	obs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 1 || obs.Entries[0].RelativeDir != "." {
		t.Fatalf("root skill: got %+v", obs.Entries)
	}
	if obs.Entries[0].Name != "Alpha" {
		t.Fatalf("name: got %q", obs.Entries[0].Name)
	}
}

func TestDiscoverDepthThreeAndOrder(t *testing.T) {
	root := t.TempDir()
	// level 1, 2, 3 all valid; a level-4 skill is out of scope
	writeSkill(t, filepath.Join(root, "skills", "a"), "A", "one")
	writeSkill(t, filepath.Join(root, "skills", "b", "two"), "Two", "two levels")
	writeSkill(t, filepath.Join(root, ".agents", "skills", "c"), "Deep", "three levels")
	writeSkill(t, filepath.Join(root, "skills", "b", "two", "four"), "Four", "too deep")
	// a nested SKILL.md inside a skill directory is not a separate Skill
	writeSkill(t, filepath.Join(root, "skills", "a", "nested"), "Nested", "inside a skill")

	obs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range obs.Entries {
		dirs = append(dirs, e.RelativeDir)
	}
	want := []string{".agents/skills/c", "skills/a", "skills/b/two"}
	if strings.Join(dirs, ",") != strings.Join(want, ",") {
		t.Fatalf("entries: got %v, want %v", dirs, want)
	}
}

func TestDiscoverSkipsAndIssues(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "skills", "ok"), "Ok", "valid")
	writeSkill(t, filepath.Join(root, "node_modules", "pkg", "skills", "dep"), "Dep", "inside node_modules")
	writeSkill(t, filepath.Join(root, "build", "x"), "Build", "inside build output")
	writeSkill(t, filepath.Join(root, ".git", "objects", "y"), "Git", "inside .git")
	// invalid entries are reported and skipped
	writeSkill(t, filepath.Join(root, "skills", "noname"), "", "")
	nameOnly := filepath.Join(root, "skills", "namenot")
	if err := os.MkdirAll(nameOnly, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nameOnly, "SKILL.md"),
		[]byte("---\ndescription: only a description\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nodesc := filepath.Join(root, "skills", "nodesc")
	if err := os.MkdirAll(nodesc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodesc, "SKILL.md"),
		[]byte("---\nname: NoDesc\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(root, "skills", "badfm", "other"), "Bad", "broken")
	if err := os.WriteFile(filepath.Join(root, "skills", "badfm", "SKILL.md"),
		[]byte("---\nname: [unclosed\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	obs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 1 || obs.Entries[0].RelativeDir != "skills/ok" {
		t.Fatalf("entries: got %+v", obs.Entries)
	}
	if len(obs.Issues) != 4 {
		t.Fatalf("issues: got %+v", obs.Issues)
	}
	var reasons []string
	for _, i := range obs.Issues {
		reasons = append(reasons, i.Reason)
	}
	joined := strings.Join(reasons, "|")
	for _, want := range []string{"missing YAML frontmatter", "frontmatter name is required", "frontmatter description is required", "invalid frontmatter"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("issues %q do not mention %q", joined, want)
		}
	}
}

func TestDiscoverDeterministic(t *testing.T) {
	root := t.TempDir()
	for i, name := range []string{"zeta", "alpha", "mid"} {
		writeSkill(t, filepath.Join(root, "skills", name), name, name)
		_ = i
	}
	first, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != len(second.Entries) {
		t.Fatalf("non-deterministic entry count: %d vs %d", len(first.Entries), len(second.Entries))
	}
	for i := range first.Entries {
		if first.Entries[i].RelativeDir != second.Entries[i].RelativeDir {
			t.Fatalf("non-deterministic order: %v vs %v", first.Entries, second.Entries)
		}
	}
}

func TestDiscoverErrors(t *testing.T) {
	if _, err := Discover(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing root: want error")
	}
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(f); err == nil {
		t.Fatal("file root: want error")
	}
}

func TestFrontmatterToleratesBOMAndCRLF(t *testing.T) {
	for _, data := range []string{
		"\ufeff---\nname: X\ndescription: Y\n---\nbody\n",
		"---\r\nname: X\r\ndescription: Y\r\n---\r\nbody\r\n",
	} {
		name, desc, err := skillFrontmatter([]byte(data))
		if err != nil {
			t.Fatalf("%q: %v", data, err)
		}
		if name != "X" || desc != "Y" {
			t.Fatalf("%q: got %q/%q", data, name, desc)
		}
	}
}

func TestDiscoverNonRegularSkillMarker(t *testing.T) {
	root := t.TempDir()

	// a symlinked SKILL.md is not followed; it is an invalid entry even
	// when its target is a perfectly valid regular file
	linkDir := filepath.Join(root, "skills", "link")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(target, []byte("---\nname: Real\ndescription: valid\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(linkDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	// a regular Skill is still discovered alongside the invalid marker
	writeSkill(t, filepath.Join(root, "skills", "ok"), "Ok", "valid")

	obs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 1 || obs.Entries[0].RelativeDir != "skills/ok" {
		t.Fatalf("entries: %+v", obs.Entries)
	}
	if len(obs.Issues) != 1 || obs.Issues[0].RelativeDir != "skills/link" {
		t.Fatalf("issues: %+v", obs.Issues)
	}
	if !strings.Contains(obs.Issues[0].Reason, "not a regular file") {
		t.Fatalf("symlinked marker reason: %q", obs.Issues[0].Reason)
	}
}

// TestDiscoverFIFOSkillMarkerNeverOpened guards the safety invariant that
// discovery never opens a non-regular SKILL.md: opening a FIFO for reading
// blocks until a writer appears, so this test would hang rather than pass if
// the marker were ever read as an ordinary file.
func TestDiscoverFIFOSkillMarkerNeverOpened(t *testing.T) {
	root := t.TempDir()
	fifoDir := filepath.Join(root, "skills", "fifo")
	if err := os.MkdirAll(fifoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(fifoDir, "SKILL.md"), 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}

	obs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 0 || len(obs.Issues) != 1 {
		t.Fatalf("FIFO marker: %+v / %+v", obs.Entries, obs.Issues)
	}
	if !strings.Contains(obs.Issues[0].Reason, "not a regular file") {
		t.Fatalf("FIFO marker reason: %q", obs.Issues[0].Reason)
	}
}
