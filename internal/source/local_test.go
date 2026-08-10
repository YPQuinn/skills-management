package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// realTempDir returns a temporary directory with symlinks resolved, the
// shape a registered Local Source location takes after normalization.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestLocalObserveStableTree(t *testing.T) {
	root := realTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	obs, err := (Local{stabilityInterval: time.Millisecond}).Observe(
		context.Background(), Locator{Kind: KindLocal, Location: root}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 1 || obs.Entries[0].RelativeDir != "skills/alpha" || obs.Entries[0].Name != "Alpha" {
		t.Fatalf("observation: %+v", obs.Entries)
	}
	if obs.Entries[0].Digest == "" || obs.Digest == "" {
		t.Fatalf("digests missing: entry %q, inventory %q", obs.Entries[0].Digest, obs.Digest)
	}
}

func TestLocalObserveAppliesSubpath(t *testing.T) {
	root := realTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	writeSkill(t, filepath.Join(root, "other", "beta"), "Beta", "two")

	obs, err := (Local{stabilityInterval: time.Millisecond}).Observe(
		context.Background(), Locator{Kind: KindLocal, Location: root, Subpath: "skills"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Entries) != 1 || obs.Entries[0].RelativeDir != "alpha" || obs.Entries[0].Name != "Alpha" {
		t.Fatalf("subpath observation: %+v", obs.Entries)
	}
}

func TestLocalObserveRejectsSymlinkEscape(t *testing.T) {
	root := realTempDir(t)
	outside := t.TempDir()
	writeSkill(t, outside, "Beta", "two")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{stabilityInterval: time.Millisecond}).Observe(
		context.Background(), Locator{Kind: KindLocal, Location: root, Subpath: "escape"}, ""); err == nil {
		t.Fatal("subpath escaping through a symlink: want error")
	}
}

// TestLocalObserveRejectsSymlinkReplacedRoot guards the physical identity
// of a registered Local Source: once the stored real root is replaced by a
// symlink pointing elsewhere, observation must fail instead of scanning the
// replacement.
func TestLocalObserveRejectsSymlinkReplacedRoot(t *testing.T) {
	base := realTempDir(t)
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	// sanity: observation works while the root is the registered real dir
	if _, err := (Local{stabilityInterval: time.Millisecond}).Observe(
		context.Background(), Locator{Kind: KindLocal, Location: root}, ""); err != nil {
		t.Fatal(err)
	}

	// swap the root for a symlink into another tree
	other := t.TempDir()
	writeSkill(t, filepath.Join(other, "skills", "beta"), "Beta", "two")
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, root); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{stabilityInterval: time.Millisecond}).Observe(
		context.Background(), Locator{Kind: KindLocal, Location: root}, ""); err == nil {
		t.Fatal("symlink-replaced root must be rejected")
	}
}
