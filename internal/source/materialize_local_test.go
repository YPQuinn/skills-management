package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// resolveTempDir returns a fully resolved temp directory, as a registered
// Local Source requires (macOS /var is a symlink to /private/var).
func resolveTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// observeLocalEntry scans a Local Source and returns the observation plus
// the entry for relativeDir ("" for the root skill).
func observeLocalEntry(t *testing.T, root, relativeDir string) (Observation, Entry) {
	t.Helper()
	obs, err := (Local{stabilityInterval: time.Millisecond}).Observe(context.Background(),
		Locator{Kind: KindLocal, Location: root}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range obs.Entries {
		if relativeDir == "" && e.RelativeDir == "." {
			return obs, e
		}
		if e.RelativeDir == relativeDir {
			return obs, e
		}
	}
	t.Fatalf("entry %q not found in %+v", relativeDir, obs.Entries)
	return Observation{}, Entry{}
}

// observeLocalIssues scans a Local Source and returns its recorded Issues.
func observeLocalIssues(t *testing.T, root string) []Issue {
	t.Helper()
	obs, err := (Local{stabilityInterval: time.Millisecond}).Observe(context.Background(),
		Locator{Kind: KindLocal, Location: root}, "")
	if err != nil {
		t.Fatal(err)
	}
	return obs.Issues
}

// materializeLocalEntry runs MaterializeEntry for a Local locator.
func materializeLocalEntry(t *testing.T, loc Locator, entry Entry, dst string) string {
	t.Helper()
	digest, err := MaterializeEntry(context.Background(), loc, "", entry, "", dst, false)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// walkKinds returns every path under root with its Lstat node kind, so
// tests can prove a tree contains no symlinks and preserves structure.
func walkKinds(t *testing.T, root string) map[string]string {
	t.Helper()
	kinds := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			kinds[rel] = "dir"
		case info.Mode().IsRegular():
			kinds[rel] = "file"
		case info.Mode()&os.ModeSymlink != 0:
			kinds[rel] = "symlink"
		default:
			kinds[rel] = "special"
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return kinds
}

func TestMaterializeLocalCopiesCompleteTree(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	if err := os.MkdirAll(filepath.Join(root, "skills", "alpha", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", "run.sh"), []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "alpha", "refs", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", "refs", "deep", "x.md"), []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .git metadata is excluded from the copy
	if err := os.MkdirAll(filepath.Join(root, "skills", "alpha", ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", ".git", "config"), []byte("git"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, entry := observeLocalEntry(t, root, "skills/alpha")
	dst := t.TempDir()
	digest := materializeLocalEntry(t, Locator{Kind: KindLocal, Location: root}, entry, dst)

	kinds := walkKinds(t, dst)
	for _, want := range []string{".", "SKILL.md", "run.sh", "empty", "refs", "refs/deep", "refs/deep/x.md"} {
		if _, ok := kinds[want]; !ok {
			t.Fatalf("materialized tree missing %q: %v", want, kinds)
		}
	}
	if _, ok := kinds[".git"]; ok {
		t.Fatal(".git must be excluded from the copy")
	}
	info, err := os.Lstat(filepath.Join(dst, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("executable bit must be preserved")
	}
	if got, err := TreeDigest(context.Background(), dst); err != nil || got != digest {
		t.Fatalf("returned digest %s does not match the tree digest %s (%v)", digest, got, err)
	}
	// a symlink-free tree must produce exactly the observed digest
	if digest != entry.Digest {
		t.Fatalf("materialized digest %s must equal the observation digest %s for a symlink-free tree", digest, entry.Digest)
	}
}

func TestMaterializeLocalRootSkill(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, root, "Root", "at root")
	if err := os.WriteFile(filepath.Join(root, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("run.sh", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	_, entry := observeLocalEntry(t, root, "")
	if entry.RelativeDir != "." {
		t.Fatalf("root entry: %+v", entry)
	}
	dst := t.TempDir()
	digest := materializeLocalEntry(t, Locator{Kind: KindLocal, Location: root}, entry, dst)

	kinds := walkKinds(t, dst)
	if kinds["link"] != "file" {
		t.Fatalf("root-skill symlink not dereferenced: %v", kinds)
	}
	if got, err := TreeDigest(context.Background(), dst); err != nil || got != digest {
		t.Fatalf("root-skill digest: %s vs %s (%v)", digest, got, err)
	}
	// the dereferenced symlink is part of the effective identity
	if digest != entry.Digest {
		t.Fatalf("root-skill materialized digest %s must equal the observation digest %s", digest, entry.Digest)
	}
}

func TestMaterializeLocalDetectsChangedSource(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")

	obs, err := (Local{stabilityInterval: time.Millisecond}).Observe(context.Background(),
		Locator{Kind: KindLocal, Location: root}, "")
	if err != nil {
		t.Fatal(err)
	}
	var entry Entry
	for _, e := range obs.Entries {
		if e.RelativeDir == "skills/alpha" {
			entry = e
		}
	}
	// the source changes after its observation
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", "SKILL.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "", entry, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("changed source: got %v, want a digest-mismatch error", err)
	}
}

func TestMaterializeLocalHonorsCancellation(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")

	_, entry := observeLocalEntry(t, root, "skills/alpha")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := MaterializeEntry(ctx, Locator{Kind: KindLocal, Location: root}, "", entry, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("cancelled materialization: got %v, want a cancellation error", err)
	}
}

func TestMaterializeEntryRejectsUnknownKind(t *testing.T) {
	_, err := MaterializeEntry(context.Background(), Locator{Kind: Kind("nope")}, "", Entry{RelativeDir: "."}, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "unsupported Source kind") {
		t.Fatalf("unknown kind: got %v, want an unsupported-kind error", err)
	}
}

func TestMaterializeEntryRejectsUnsafeInventoryPath(t *testing.T) {
	for _, relativeDir := range []string{"../escape", "skills/../escape", "/absolute", "skills//alpha"} {
		_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal}, "",
			Entry{RelativeDir: relativeDir}, "", t.TempDir(), false)
		if err == nil || !strings.Contains(err.Error(), "invalid Inventory entry path") {
			t.Errorf("RelativeDir %q: got %v, want a path error", relativeDir, err)
		}
	}
}

// TestMaterializeLocalEmptyDirectorySymlink proves a directory symlink whose
// target is empty materializes the link path as a directory in the
// destination, the observed digest equals the materialized digest, and the
// empty linked directory exists.
func TestMaterializeLocalEmptyDirectorySymlink(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	// an empty target directory
	if err := os.MkdirAll(filepath.Join(root, "skills", "alpha", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	// a symlink to the empty directory
	if err := os.Symlink("empty", filepath.Join(root, "skills", "alpha", "link-empty")); err != nil {
		t.Fatal(err)
	}

	_, entry := observeLocalEntry(t, root, "skills/alpha")
	dst := t.TempDir()
	digest := materializeLocalEntry(t, Locator{Kind: KindLocal, Location: root}, entry, dst)

	// the materialized digest must equal the observed entry digest
	if digest != entry.Digest {
		t.Fatalf("materialized digest %s != observed digest %s", digest, entry.Digest)
	}
	// the empty linked directory must exist as a directory
	info, err := os.Lstat(filepath.Join(dst, "link-empty"))
	if err != nil {
		t.Fatalf("empty linked directory was not materialized: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("link-empty must be a directory, got mode %v", info.Mode())
	}
	// re-walking the materialized tree must reproduce the same digest
	if got, err := TreeDigest(context.Background(), dst); err != nil || got != digest {
		t.Fatalf("re-walked digest %s != materialized digest %s (%v)", got, digest, err)
	}
}
