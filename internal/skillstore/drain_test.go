package skillstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestDrainTreeRefusesForeignIdentity proves cleanup only drains the object
// that was opened and verified: when the caller's retained identity does
// not match the current object, drainTree refuses and the foreign non-empty
// content survives.
func TestDrainTreeRefusesForeignIdentity(t *testing.T) {
	dir := t.TempDir()
	parent, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err := os.MkdirAll(filepath.Join(dir, "tree", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tree", "sub", "f.txt"), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := drainTree(parent, "tree", fileID{dev: 1, ino: 2}); err == nil {
		t.Fatal("draining an object with a mismatched identity: want error")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "tree", "sub", "f.txt")); err != nil || string(data) != "foreign" {
		t.Fatalf("foreign content must survive: %q, %v", data, err)
	}
}

// TestDrainTreeRemovesPinnedTree proves a normal drain removes the whole
// tree, including nested directories and files, through pinned handles.
func TestDrainTreeRemovesPinnedTree(t *testing.T) {
	dir := t.TempDir()
	parent, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err := os.MkdirAll(filepath.Join(dir, "tree", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tree", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tree", "sub", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := drainTree(parent, "tree", fileID{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "tree")); !os.IsNotExist(err) {
		t.Fatalf("tree must be removed, stat: %v", err)
	}
}

// TestDrainTreeIsIdempotent proves an absent tree is a successful no-op,
// matching the removed RemoveAll callers.
func TestDrainTreeIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	parent, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err := drainTree(parent, "absent", fileID{}); err != nil {
		t.Fatalf("draining an absent tree: %v", err)
	}
}

// TestDrainTreeAbsentWithExpectedIdentityFails proves a caller that proved
// a specific object may never certify its cleanup from an absent name: the
// object vanished between proof and drain, so cleanup is reported as
// ErrAmbiguous instead of a silent success.
func TestDrainTreeAbsentWithExpectedIdentityFails(t *testing.T) {
	dir := t.TempDir()
	parent, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	err = drainTree(parent, "absent", fileID{dev: 1, ino: 2})
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("absent tree with a proven identity: got %v, want ErrAmbiguous", err)
	}
}

// TestDrainTreeDirectoryChildUsesPinnedIdentity proves the recursive drain
// of a directory child carries the identity sampled from the child's own
// Lstat, exactly as drainChild now does: after the slot is swapped for a
// fresh foreign directory, the drain called with the pinned identity must
// refuse the foreign directory and preserve it, instead of silently
// draining the replacement.
func TestDrainTreeDirectoryChildUsesPinnedIdentity(t *testing.T) {
	dir := t.TempDir()
	parent, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "f.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	// capture the identity of the child directory from its own Lstat, as
	// drainChild does before recursing
	info, err := parent.Lstat("sub")
	if err != nil {
		t.Fatal(err)
	}
	subID, err := fileIDOf(info)
	if err != nil {
		t.Fatal(err)
	}
	// a foreign directory replaces the child after the identity was sampled
	if err := os.Rename(filepath.Join(dir, "sub"), filepath.Join(dir, "sub.real")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "deeper", "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// the drain with the pinned identity must refuse the foreign directory
	err = drainTree(parent, "sub", subID)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign directory child: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "sub", "deeper", "x")); err != nil {
		t.Fatal("the foreign directory content must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(dir, "sub.real", "f.txt")); err != nil {
		t.Fatal("the original child directory must survive")
	}
}

// TestDrainChildDirectorySwapAfterSampleIsAmbiguous exercises the real
// ReadDir→Lstat→drainChild window that direct drainTree calls cannot reach:
// a byte-identical foreign directory swapped in after the child was
// classified and sampled is refused by the recursive drain's identity check
// and preserved, and the original child survives.
func TestDrainChildDirectorySwapAfterSampleIsAmbiguous(t *testing.T) {
	dir := t.TempDir()
	parent, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err := os.MkdirAll(filepath.Join(dir, "tree", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tree", "sub", "f.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, id, err := openPinnedChild(parent, "tree")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { testDrainChildHook = nil }()
	testDrainChildHook = func(p *os.Root, name string) {
		if name != "sub" {
			return
		}
		real := filepath.Join(dir, "tree", "sub.real")
		if err := os.Rename(filepath.Join(dir, "tree", "sub"), real); err != nil {
			t.Fatal(err)
		}
		// a byte-identical foreign directory takes the slot
		if err := copyTreeForTest(filepath.Join(dir, "tree", "sub"), real); err != nil {
			t.Fatal(err)
		}
	}

	err = drainTree(parent, "tree", id)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign directory child in the drain window: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(dir, "tree", "sub", "f.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the foreign directory content must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(dir, "tree", "sub.real", "f.txt")); err != nil {
		t.Fatal("the original child directory must survive")
	}
}

// TestDrainChildFileSwapAfterOpenIsAmbiguous proves the file-drain window:
// a regular file whose identity changed between the Lstat classification and
// the pinned open is reported as ErrAmbiguous and preserved.
func TestDrainChildFileSwapAfterOpenIsAmbiguous(t *testing.T) {
	dir := t.TempDir()
	parent, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err := os.MkdirAll(filepath.Join(dir, "tree"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tree", "f.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, id, err := openPinnedChild(parent, "tree")
	if err != nil {
		t.Fatal(err)
	}
	swap := t.TempDir()
	if err := os.WriteFile(filepath.Join(swap, "f.txt"), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { testDrainChildHook = nil }()
	testDrainChildHook = func(p *os.Root, name string) {
		if name != "f.txt" {
			return
		}
		if err := os.RemoveAll(filepath.Join(dir, "tree", "f.txt")); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(swap, "f.txt"), filepath.Join(dir, "tree", "f.txt")); err != nil {
			t.Fatal(err)
		}
	}

	err = drainTree(parent, "tree", id)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign regular file in the drain window: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(dir, "tree", "f.txt")); rerr != nil || string(data) != "foreign" {
		t.Fatalf("the foreign file must be preserved: %q, %v", data, rerr)
	}
}
