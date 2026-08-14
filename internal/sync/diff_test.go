package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree builds one Skill-like tree under dir.
func writeTree(t *testing.T, dir string, files map[string]string, execs map[string]bool) {
	t.Helper()
	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if execs[path] {
			mode = 0o755
		}
		if err := os.WriteFile(full, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
}

func snapshotDir(t *testing.T, dir string) Tree {
	t.Helper()
	tree, err := SnapshotTree(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// TestCompareClassifiesChanges locks the five change kinds: add, delete,
// content, exec, and node-type.
func TestCompareClassifiesChanges(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeTree(t, a, map[string]string{
		"SKILL.md":        "hello\n",
		"same.txt":        "same\n",
		"gone.txt":        "bye\n",
		"content.txt":     "old\n",
		"exec.sh":         "#!/bin/sh\necho hi\n",
		"to-dir":          "was a file\n",
		"dir/deleted.txt": "x\n",
	}, nil)
	writeTree(t, b, map[string]string{
		"SKILL.md":     "hello\n",
		"same.txt":     "same\n",
		"added.txt":    "new\n",
		"content.txt":  "new\n",
		"exec.sh":      "#!/bin/sh\necho hi\n",
		"to-dir":       "dir now\n",
		"dir/kept.txt": "y\n",
	}, map[string]bool{"exec.sh": true})
	// b/to-dir is a directory: create it after removing the file.
	if err := os.Remove(filepath.Join(b, "to-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(b, "to-dir", "child"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := Compare("a", "b", snapshotDir(t, a), snapshotDir(t, b))
	byPath := map[string]Entry{}
	for _, e := range c.Entries {
		byPath[e.Path] = e
	}
	expect := func(path string, kinds ...string) {
		t.Helper()
		e, ok := byPath[path]
		if !ok {
			t.Fatalf("no entry for %q in %+v", path, c.Entries)
		}
		if len(e.Changes) != len(kinds) {
			t.Fatalf("%s changes = %v, want %v", path, e.Changes, kinds)
		}
		for i, k := range kinds {
			if e.Changes[i] != k {
				t.Fatalf("%s changes = %v, want %v", path, e.Changes, kinds)
			}
		}
	}
	expect("added.txt", ChangeAdd)
	expect("gone.txt", ChangeDelete)
	expect("content.txt", ChangeContent)
	expect("exec.sh", ChangeExec)
	expect("to-dir", ChangeNodeType)
	expect("dir/deleted.txt", ChangeDelete)
	expect("dir/kept.txt", ChangeAdd)
	if _, ok := byPath["SKILL.md"]; ok {
		t.Fatal("unchanged SKILL.md must not appear")
	}
	if _, ok := byPath["same.txt"]; ok {
		t.Fatal("unchanged same.txt must not appear")
	}
	// The content change renders unified text.
	e := byPath["content.txt"]
	if e.Text == nil || !strings.Contains(e.Text.Unified, "-old") || !strings.Contains(e.Text.Unified, "+new") {
		t.Fatalf("content text diff missing: %+v", e.Text)
	}
	// The exec-only change carries no text.
	if byPath["exec.sh"].Text != nil {
		t.Fatal("exec-only change must not carry text")
	}
}

// TestCompareTextLimits locks the decision-05 presentation rule: binary,
// undecodable, and oversized files expose digests instead of unified text.
func TestCompareTextLimits(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeTree(t, a, map[string]string{"binary.bin": "old", "big.txt": strings.Repeat("x\n", 1)}, nil)
	writeTree(t, b, map[string]string{"binary.bin": "new", "big.txt": strings.Repeat("y\n", 1)}, nil)
	// Binary content with a NUL byte.
	if err := os.WriteFile(filepath.Join(a, "binary.bin"), []byte("a\x00b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "binary.bin"), []byte("c\x00d"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Oversized text file.
	big := strings.Repeat("line\n", TextMaxLines+1)
	if err := os.WriteFile(filepath.Join(a, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "big.txt"), []byte(big+"tail\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := Compare("a", "b", snapshotDir(t, a), snapshotDir(t, b))
	for _, e := range c.Entries {
		if e.Changes[0] != ChangeContent {
			continue
		}
		if e.Text != nil {
			t.Fatalf("%s must not render text (binary or oversized): %+v", e.Path, e.Text)
		}
		if e.From == nil || e.To == nil || e.From.Digest == "" || e.To.Digest == "" {
			t.Fatalf("%s must expose both digests: %+v", e.Path, e)
		}
	}
}

// TestUnifiedText locks the bounded diff rendering on a real change.
func TestUnifiedText(t *testing.T) {
	a := "alpha\nbeta\ngamma\ndelta\n"
	b := "alpha\nBETA\ngamma\ndelta\nepsilon\n"
	got := UnifiedText(a, b)
	if !strings.Contains(got, "@@ -1,5 +1,6 @@") {
		t.Fatalf("unified hunk header missing:\n%s", got)
	}
	if !strings.Contains(got, " alpha\n-beta\n+BETA\n gamma\n") {
		t.Fatalf("unified body wrong:\n%s", got)
	}
	if !strings.Contains(got, "+epsilon") {
		t.Fatalf("addition missing:\n%s", got)
	}
	// Whole-file replacement fallback for identical-in-nothing files.
	got = UnifiedText("a\n", "b\n")
	if !strings.Contains(got, "-a\n") || !strings.Contains(got, "+b\n") {
		t.Fatalf("replace-all fallback wrong:\n%s", got)
	}
	// Empty-to-content and content-to-empty render too.
	if got := UnifiedText("", "x\n"); !strings.Contains(got, "+x") {
		t.Fatalf("empty-to-content wrong:\n%s", got)
	}
	if got := UnifiedText("x\n", ""); !strings.Contains(got, "-x") {
		t.Fatalf("content-to-empty wrong:\n%s", got)
	}
}

// TestUnifiedTextNoTrailingNewline locks the last-line handling when the
// final line carries no trailing newline: a content change, an addition,
// and a deletion must all still render the last line instead of dropping
// it (diff(1) semantics: only the newline itself is an empty terminator
// line).
func TestUnifiedTextNoTrailingNewline(t *testing.T) {
	// Content change in the newline-less last line.
	got := UnifiedText("alpha\nbeta", "alpha\nBETA")
	if !strings.Contains(got, "-beta") || !strings.Contains(got, "+BETA") {
		t.Fatalf("last-line content change dropped:\n%s", got)
	}
	// Addition of a newline-less last line.
	got = UnifiedText("alpha\n", "alpha\nbeta")
	if !strings.Contains(got, "+beta") {
		t.Fatalf("last-line addition dropped:\n%s", got)
	}
	// Deletion of a newline-less last line.
	got = UnifiedText("alpha\nbeta", "alpha\n")
	if !strings.Contains(got, "-beta") {
		t.Fatalf("last-line deletion dropped:\n%s", got)
	}
	// A single newline-less line still diffes against empty content.
	if got := UnifiedText("only", ""); !strings.Contains(got, "-only") {
		t.Fatalf("single newline-less line dropped:\n%s", got)
	}
	if got := UnifiedText("", "only"); !strings.Contains(got, "+only") {
		t.Fatalf("single newline-less addition dropped:\n%s", got)
	}
}

// TestFilterEntries locks path filtering at and below one path.
func TestFilterEntries(t *testing.T) {
	c := Comparison{From: "a", To: "b", Entries: []Entry{
		{Path: "dir/file.txt", Changes: []string{ChangeContent}},
		{Path: "dir/deep/other.txt", Changes: []string{ChangeAdd}},
		{Path: "other.txt", Changes: []string{ChangeDelete}},
		{Path: "dir2/out.txt", Changes: []string{ChangeExec}},
	}}
	got := c.FilterEntries("dir")
	if len(got.Entries) != 2 {
		t.Fatalf("filtered entries = %+v", got.Entries)
	}
	for _, e := range got.Entries {
		if e.Path != "dir/file.txt" && e.Path != "dir/deep/other.txt" {
			t.Fatalf("unexpected filtered entry %q", e.Path)
		}
	}
	if got.FilterEntries("").Entries == nil {
		t.Fatal("empty filter must return everything")
	}
}
