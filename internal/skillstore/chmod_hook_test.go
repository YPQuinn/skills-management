package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestStageDirChmodSymlinkSwapDoesNotTouchTarget proves the directory mode
// is applied through the pinned child's own handle: a symlink swapped onto
// the staged directory name before the fchmod never redirects the mode to
// the foreign target, the operation fails, and the foreign target keeps its
// mode and content.
func TestStageDirChmodSymlinkSwapDoesNotTouchTarget(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"sub/inner.txt": "x"})
	foreign := t.TempDir()
	if err := os.WriteFile(filepath.Join(foreign, "f.txt"), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	defer func() { testBeforeChmodDirHook = nil }()
	testBeforeChmodDirHook = func(parent *os.Root, name string) {
		if name != "sub" {
			return
		}
		tree := s.stagedTreeDir(1)
		if err := os.Rename(filepath.Join(tree, "sub"), filepath.Join(tree, "sub.real")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(foreign, filepath.Join(tree, "sub")); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Stage(context.Background(), 1, m.dir, m.digest, false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("symlink swap before the directory chmod: got %v, want ErrAmbiguous", err)
	}
	info, err := os.Lstat(foreign)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("foreign target mode must be unchanged: got %o, want 700", info.Mode().Perm())
	}
	if data, rerr := os.ReadFile(filepath.Join(foreign, "f.txt")); rerr != nil || string(data) != "foreign" {
		t.Fatalf("foreign target content must be unchanged: %q, %v", data, rerr)
	}
}

// TestStageFileChmodSymlinkSwapDoesNotTouchTarget proves the file mode is
// applied through the still-open created handle: a symlink swapped onto the
// staged file name before the chmod never redirects the mode to the foreign
// target, the operation fails, and the foreign target keeps its mode and
// content.
func TestStageFileChmodSymlinkSwapDoesNotTouchTarget(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	foreign := t.TempDir()
	target := filepath.Join(foreign, "target.txt")
	if err := os.WriteFile(target, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() { testBeforeChmodFileHook = nil }()
	testBeforeChmodFileHook = func(parent *os.Root, name string) {
		if name != "SKILL.md" {
			return
		}
		tree := s.stagedTreeDir(1)
		if err := os.Rename(filepath.Join(tree, "SKILL.md"), filepath.Join(tree, "SKILL.md.real")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(tree, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Stage(context.Background(), 1, m.dir, m.digest, false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("symlink swap before the file chmod: got %v, want ErrAmbiguous", err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("foreign target mode must be unchanged: got %o, want 600", info.Mode().Perm())
	}
	if data, rerr := os.ReadFile(target); rerr != nil || string(data) != "foreign" {
		t.Fatalf("foreign target content must be unchanged: %q, %v", data, rerr)
	}
}
