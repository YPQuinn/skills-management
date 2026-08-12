package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// assertIssue contains wantSubstr in one of the observation Issues for the
// given relative directory.
func assertIssue(t *testing.T, issues []Issue, relativeDir, wantSubstr string) {
	t.Helper()
	for _, is := range issues {
		if is.RelativeDir == relativeDir && strings.Contains(is.Reason, wantSubstr) {
			return
		}
	}
	t.Fatalf("no Issue for %q containing %q in %+v", relativeDir, wantSubstr, issues)
}

func TestMaterializeLocalDereferencesInternalSymlinks(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "alpha", "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", "tools", "helper.sh"), []byte("#!/bin/sh\nhelper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a symlink to a file and a symlink to a directory
	if err := os.Symlink("run.sh", filepath.Join(root, "skills", "alpha", "link-file")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tools", filepath.Join(root, "skills", "alpha", "link-dir")); err != nil {
		t.Fatal(err)
	}

	_, entry := observeLocalEntry(t, root, "skills/alpha")
	dst := t.TempDir()
	digest := materializeLocalEntry(t, Locator{Kind: KindLocal, Location: root}, entry, dst)

	kinds := walkKinds(t, dst)
	for p, want := range map[string]string{
		"link-file":          "file",
		"link-dir":           "dir",
		"link-dir/helper.sh": "file",
	} {
		if kinds[p] != want {
			t.Fatalf("%q: got %q, want a dereferenced %s (kinds: %v)", p, kinds[p], want, kinds)
		}
	}
	if kinds["link-dir"] == "symlink" {
		t.Fatal("directory symlink must be dereferenced")
	}
	data, err := os.ReadFile(filepath.Join(dst, "link-file"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "#!/bin/sh\n" {
		t.Fatalf("dereferenced file content: %q", data)
	}
	// the copy is self-contained: not a single symlink survives
	for p, k := range kinds {
		if k == "symlink" {
			t.Fatalf("symlink %q survived materialization", p)
		}
	}
	// the observation digest is the effective materialized identity: a
	// newly imported symlink Skill must never diverge from its Source
	if digest != entry.Digest {
		t.Fatalf("materialized digest %s must equal the observation digest %s for a dereferenced symlink tree", digest, entry.Digest)
	}
}

// TestObservationRejectsEscapingSymlink proves the check itself records the
// Skill as invalid, and TestMaterializeLocalRejectsEscapingSymlink proves
// the materializer still rejects a crafted entry directly (defense in
// depth).
func TestObservationRejectsEscapingSymlink(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	outside := resolveTempDir(t)
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "skills", "alpha", "leak")); err != nil {
		t.Fatal(err)
	}

	issues := observeLocalIssues(t, root)
	assertIssue(t, issues, "skills/alpha", "escapes the Skill root")
}

func TestMaterializeLocalRejectsEscapingSymlink(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	outside := resolveTempDir(t)
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "skills", "alpha", "leak")); err != nil {
		t.Fatal(err)
	}

	_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "",
		Entry{RelativeDir: "skills/alpha", Digest: "stale"}, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "escapes the Skill root") {
		t.Fatalf("escaping symlink: got %v, want an escape error", err)
	}
}

func TestObservationRejectsBrokenSymlink(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	if err := os.Symlink("missing-target", filepath.Join(root, "skills", "alpha", "dangling")); err != nil {
		t.Fatal(err)
	}

	issues := observeLocalIssues(t, root)
	assertIssue(t, issues, "skills/alpha", "missing")
}

func TestMaterializeLocalRejectsBrokenSymlink(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	if err := os.Symlink("missing-target", filepath.Join(root, "skills", "alpha", "dangling")); err != nil {
		t.Fatal(err)
	}

	_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "",
		Entry{RelativeDir: "skills/alpha", Digest: "stale"}, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "broken symlink") {
		t.Fatalf("broken symlink: got %v, want a broken-link error", err)
	}
}

func TestObservationRejectsSymlinkLoop(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	// a link to its own directory can never be dereferenced finitely
	if err := os.Symlink(".", filepath.Join(root, "skills", "alpha", "self")); err != nil {
		t.Fatal(err)
	}

	issues := observeLocalIssues(t, root)
	assertIssue(t, issues, "skills/alpha", "symlink loop")
}

func TestMaterializeLocalRejectsSymlinkLoop(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	if err := os.Symlink(".", filepath.Join(root, "skills", "alpha", "self")); err != nil {
		t.Fatal(err)
	}

	_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "",
		Entry{RelativeDir: "skills/alpha", Digest: "stale"}, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "symlink loop") {
		t.Fatalf("symlink loop: got %v, want a loop error", err)
	}
}

func TestObservationRejectsSpecialNode(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	fifo := filepath.Join(root, "skills", "alpha", "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}

	issues := observeLocalIssues(t, root)
	assertIssue(t, issues, "skills/alpha", "cannot be imported")
}

func TestMaterializeLocalRejectsSpecialNode(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	fifo := filepath.Join(root, "skills", "alpha", "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}

	_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "",
		Entry{RelativeDir: "skills/alpha", Digest: "stale"}, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "special node") {
		t.Fatalf("FIFO: got %v, want a special-node error", err)
	}
}

func TestMaterializeLocalRejectsLFSPointer(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	pointer := "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize 12345\n"
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", "data.bin"), []byte(pointer), 0o644); err != nil {
		t.Fatal(err)
	}

	_, entry := observeLocalEntry(t, root, "skills/alpha")
	_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "", entry, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "LFS pointer") {
		t.Fatalf("LFS pointer: got %v, want an LFS error", err)
	}
}

// TestMaterializeLocalExcludesDotGitFile proves a linked-worktree-style
// .git file (a regular file, not a directory) is excluded from observation
// and materialization exactly like .git directories.
func TestMaterializeLocalExcludesDotGitFile(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	gitFile := filepath.Join(root, "skills", "alpha", ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: ../.git/worktrees/alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	obs, entry := observeLocalEntry(t, root, "skills/alpha")
	if len(obs.Issues) != 0 {
		t.Fatalf("a .git file must not invalidate the entry: %+v", obs.Issues)
	}
	dst := t.TempDir()
	digest := materializeLocalEntry(t, Locator{Kind: KindLocal, Location: root}, entry, dst)

	kinds := walkKinds(t, dst)
	if _, ok := kinds[".git"]; ok {
		t.Fatal(".git file must be excluded from the copy")
	}
	if digest != entry.Digest {
		t.Fatalf("digest with .git file: materialized %s vs observed %s", digest, entry.Digest)
	}
}

// TestMaterializeLocalExcludesDotGitSymlink covers a .git symlink: it is
// metadata, never content, so it is excluded rather than dereferenced.
func TestMaterializeLocalExcludesDotGitSymlink(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	if err := os.Symlink("elsewhere", filepath.Join(root, "skills", "alpha", ".git")); err != nil {
		t.Fatal(err)
	}

	obs, entry := observeLocalEntry(t, root, "skills/alpha")
	if len(obs.Issues) != 0 {
		t.Fatalf("a .git symlink must not invalidate the entry: %+v", obs.Issues)
	}
	dst := t.TempDir()
	digest := materializeLocalEntry(t, Locator{Kind: KindLocal, Location: root}, entry, dst)

	kinds := walkKinds(t, dst)
	if _, ok := kinds[".git"]; ok {
		t.Fatal(".git symlink must be excluded from the copy")
	}
	if digest != entry.Digest {
		t.Fatalf("digest with .git symlink: materialized %s vs observed %s", digest, entry.Digest)
	}
}

func TestMaterializeLocalRejectsSymlinkedDestinationRoot(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	_, entry := observeLocalEntry(t, root, "skills/alpha")
	outside := t.TempDir()
	dst := filepath.Join(t.TempDir(), "destination")
	if err := os.Symlink(outside, dst); err != nil {
		t.Fatal(err)
	}

	_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "", entry, "", dst, false)
	if err == nil || !strings.Contains(err.Error(), "destination is not a directory") {
		t.Fatalf("symlinked destination: got %v, want an error", err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("symlink target must remain untouched: %v, %v", entries, readErr)
	}
}

// TestObservationRejectsNonCanonicalPaths would need a crafted Local tree
// with ".." names, which the OS forbids; the equivalent Git-side validation
// is pinned in effective_test.go.
