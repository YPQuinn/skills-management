package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSkillDirRejectsUnsafeSlugs pins the decision-03 slug grammar at the
// Store path boundary: no separator, dot name, or .skillctl target can ever
// be joined into a Store path.
func TestSkillDirRejectsUnsafeSlugs(t *testing.T) {
	s := newStore(t)
	bad := []string{
		"",
		".",
		"..",
		"../escape",
		"a/b",
		"a\\b",
		"a..b",
		"-a",
		"a-",
		"a--b",
		"A",
		"a b",
		".skillctl",
		"a.skillctl",
		strings.Repeat("a", 65),
	}
	for _, slug := range bad {
		if _, err := s.SkillDir(slug); err == nil {
			t.Errorf("SkillDir(%q): want error", slug)
		}
	}
	good := []string{"a", "alpha", "a-b", "a1", "x-1-y", strings.Repeat("a", 64)}
	for _, slug := range good {
		dir, err := s.SkillDir(slug)
		if err != nil {
			t.Errorf("SkillDir(%q): %v", slug, err)
			continue
		}
		if filepath.Base(dir) != slug {
			t.Errorf("SkillDir(%q) = %q, want the slug as base", slug, dir)
		}
	}
}

// TestSkillDirRejectsSlugInRecoveryOfPersistedOperation proves the same
// validation guards slugs read back from the journal: a tampered operation
// can never make recovery touch a path outside the Store.
func TestSkillDirRejectsSlugInRecoveryOfPersistedOperation(t *testing.T) {
	s := newStore(t)
	op := Operation{ID: 1, Slug: "../escape", Kind: KindImport, NewDigest: "d"}
	if err := s.Restore(context.Background(), op); err == nil {
		t.Fatal("restore with an unsafe slug: want error")
	}
	if err := s.Finalize(context.Background(), op); err == nil {
		t.Fatal("finalize with an unsafe slug: want error")
	}
	if _, statErr := os.Lstat(filepath.Join(s.Root, "..", "escape")); !os.IsNotExist(statErr) {
		t.Fatalf("recovery must not touch paths outside the Store, stat: %v", statErr)
	}
}

func TestEnsureLayoutRejectsSymlinkedInternalDirectory(t *testing.T) {
	s := newStore(t)
	outside := t.TempDir()
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, s.InternalDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureLayout(); err == nil {
		t.Fatal("symlinked internal directory: want error")
	}
	if _, err := os.Lstat(filepath.Join(outside, "staging")); !os.IsNotExist(err) {
		t.Fatalf("layout creation must not escape through the symlink: %v", err)
	}
}

// TestEnsureLayoutRejectsRelativeSymlinkInternalDir proves a relative
// .skillctl symlink raced into place can never redirect layout creation
// into a Skill subtree: the one-component creation and pin of .skillctl
// refuses the link and no child directory is created outside the Store.
func TestEnsureLayoutRejectsRelativeSymlinkInternalDir(t *testing.T) {
	s := newStore(t)
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(s.Root), "outside")
	if err := os.Symlink("../outside", s.InternalDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureLayout(); err == nil {
		t.Fatal("relative symlinked internal directory: want error")
	}
	// the relative link resolves beside the Store root; nothing may be
	// created through it
	if _, err := os.Lstat(filepath.Join(outside, "staging")); !os.IsNotExist(err) {
		t.Fatalf("layout creation must not escape through the relative symlink: %v", err)
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Fatalf("the link target must not be created: %v", err)
	}
}

// TestStageRequiresAbsentOperationDir proves Stage creates the operation
// directory exclusively: a pre-existing directory is contradictory evidence
// and is preserved with ErrAmbiguous instead of being removed.
func TestStageRequiresAbsentOperationDir(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.opDir(1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.opDir(1), "foreign.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := s.Stage(context.Background(), 1, m.dir, m.digest, false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("pre-existing operation directory: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.opDir(1), "foreign.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("pre-existing operation directory must be preserved: %q, %v", data, rerr)
	}
}

func TestRenameNoReplaceAt(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "p1")
	p2 := filepath.Join(dir, "p2")
	if err := os.MkdirAll(p1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p2, 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := os.OpenRoot(p1)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := os.OpenRoot(p2)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	src := filepath.Join(p1, "src")
	dst := filepath.Join(p2, "dst")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplaceAt(a, "src", b, "dst"); !errors.Is(err, errDestinationExists) {
		t.Fatalf("rename over existing: got %v, want errDestinationExists", err)
	}
	// the destination is untouched and the source still exists
	if data, err := os.ReadFile(dst); err != nil || string(data) != "old" {
		t.Fatalf("destination after refused rename: %q, %v", data, err)
	}
	if _, err := os.Lstat(src); err != nil {
		t.Fatalf("source after refused rename: %v", err)
	}
	// multi-component names are refused outright
	if err := renameNoReplaceAt(a, "x/y", b, "z"); err == nil {
		t.Fatal("multi-component name: want error")
	}
	// renaming onto an absent destination succeeds
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplaceAt(a, "src", b, "dst"); err != nil {
		t.Fatalf("rename onto absent destination: %v", err)
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "new" {
		t.Fatalf("destination after rename: %q, %v", data, err)
	}
}
