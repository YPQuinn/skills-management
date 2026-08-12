package skillstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"skillctl/internal/source"
)

// materialized is a directory tree plus its canonical digest, standing in
// for what the materializer hands to the application.
type materialized struct {
	dir    string
	digest string
}

// buildMaterialized creates a tree from path→content entries and returns
// it with its canonical digest.
func buildMaterialized(t *testing.T, files map[string]string) materialized {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return materialized{dir: dir, digest: digest}
}

// sparseFile creates a file with the given logical size using no disk.
func sparseFile(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStageCopiesAndVerifies(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{
		"SKILL.md":     "---\nname: A\ndescription: one\n---\n",
		"tools/run.sh": "#!/bin/sh\n",
	})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}

	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if staged.opID != op.ID || staged.digest != m.digest || staged.treeID == (fileID{}) || staged.opDirID == (fileID{}) {
		t.Fatalf("staged proof: got %+v, want the operation's digest and physical identities", staged)
	}
	data, err := os.ReadFile(filepath.Join(s.stagedTreeDir(1), "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "---\nname: A\ndescription: one\n---\n" {
		t.Fatalf("staged content: %q", data)
	}
	if _, err := os.Lstat(filepath.Join(s.stagedTreeDir(1), "tools", "run.sh")); err != nil {
		t.Fatal(err)
	}
}

func TestStageRejectsDigestMismatch(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})

	_, err := s.Stage(context.Background(), 1, m.dir, "wrong-digest", false)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("digest mismatch: got %v, want a mismatch error", err)
	}
	if _, statErr := os.Lstat(s.opDir(1)); !os.IsNotExist(statErr) {
		t.Fatalf("partial staging must be removed, stat: %v", statErr)
	}
}

func TestStageEnforcesFileCountLimit(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	for i := 0; i <= MaxFilesPerSkill; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Stage(context.Background(), 1, dir, digest, false)
	if err == nil || !strings.Contains(err.Error(), "file limit") {
		t.Fatalf("file-count guard: got %v, want a limit error", err)
	}
}

func TestStageAllowLargeBypassesCountLimit(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	for i := 0; i <= MaxFilesPerSkill; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stage(context.Background(), 1, dir, digest, true); err != nil {
		t.Fatalf("allow-large: %v", err)
	}
}

func TestStageEnforcesPerFileLimit(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: A\ndescription: one\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sparseFile(t, filepath.Join(dir, "big.bin"), MaxBytesPerFile+1)
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Stage(context.Background(), 1, dir, digest, false)
	if err == nil || !strings.Contains(err.Error(), "per-file limit") {
		t.Fatalf("per-file guard: got %v, want a limit error", err)
	}
	// allow-large bypasses the guard; the digest still verifies
	if _, err := s.Stage(context.Background(), 2, dir, digest, true); err != nil {
		t.Fatalf("allow-large per-file: %v", err)
	}
}

func TestStageEnforcesTotalLimit(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: A\ndescription: one\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// each file is below the per-file limit; together they exceed the total
	sparseFile(t, filepath.Join(dir, "a.bin"), MaxBytesPerSkill/2)
	sparseFile(t, filepath.Join(dir, "b.bin"), MaxBytesPerSkill/2)
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Stage(context.Background(), 1, dir, digest, false)
	if err == nil || !strings.Contains(err.Error(), "total limit") {
		t.Fatalf("total guard: got %v, want a limit error", err)
	}
}

func TestStageRejectsSymlinkEvenWithAllowLarge(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: A\ndescription: one\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("SKILL.md", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Stage(context.Background(), 1, dir, digest, true)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink in staged input: got %v, want a symlink error", err)
	}
}

func TestStageRejectsSpecialNode(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: A\ndescription: one\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Stage(context.Background(), 1, dir, digest, true)
	if err == nil || !strings.Contains(err.Error(), "FIFO") {
		t.Fatalf("FIFO in staged input: got %v, want a special-node error", err)
	}
}

func TestStageSkipsDotGit(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: A\ndescription: one\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "objects", "x"), []byte("git"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stage(context.Background(), 1, dir, digest, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(s.stagedTreeDir(1), ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git must be skipped in staging, stat: %v", err)
	}
}

// TestStageSkipsDotGitFile proves a linked-worktree-style .git file (a
// regular file, not a directory) is excluded from staging exactly like a
// .git directory.
func TestStageSkipsDotGitFile(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: A\ndescription: one\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../.git/worktrees/alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stage(context.Background(), 1, dir, digest, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(s.stagedTreeDir(1), ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git file must be skipped in staging, stat: %v", err)
	}
}

func TestEnsureLayoutCreatesInternalDirs(t *testing.T) {
	s := newStore(t)
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"staging", "baselines", "previous", "recovery"} {
		info, err := os.Lstat(filepath.Join(s.InternalDir(), d))
		if err != nil || !info.IsDir() {
			t.Fatalf("internal dir %q: %v", d, err)
		}
	}
}
