package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRootedWriteFileChmodSymlinkSwapDoesNotTouchTarget proves the file
// mode is applied through the still-open created handle: a symlink swapped
// onto the name before the chmod never redirects the mode to the foreign
// target, and the write fails instead of reporting success.
func TestRootedWriteFileChmodSymlinkSwapDoesNotTouchTarget(t *testing.T) {
	dir := t.TempDir()
	dst, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
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
		if name != "f.txt" {
			return
		}
		if err := os.Rename(filepath.Join(dir, "f.txt"), filepath.Join(dir, "f.txt.real")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "f.txt")); err != nil {
			t.Fatal(err)
		}
	}

	err = rootedWriteFile(context.Background(), dst, "f.txt", []byte("data"), 0o644)
	if err == nil {
		t.Fatal("a symlink swapped onto the name before the chmod must fail the write")
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

// TestRootedMkdirChmodSymlinkSwapDoesNotTouchTarget proves the directory
// mode is applied through the pinned child's own handle: a symlink swapped
// onto the name before the fchmod never redirects the mode to the foreign
// target, and the create fails instead of reporting success.
func TestRootedMkdirChmodSymlinkSwapDoesNotTouchTarget(t *testing.T) {
	dir := t.TempDir()
	dst, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
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
		if err := os.Rename(filepath.Join(dir, "sub"), filepath.Join(dir, "sub.real")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(foreign, filepath.Join(dir, "sub")); err != nil {
			t.Fatal(err)
		}
	}

	child, err := rootedMkdir(dst, "sub", 0o755)
	if err == nil {
		child.Close()
		t.Fatal("a symlink swapped onto the name before the fchmod must fail the create")
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
