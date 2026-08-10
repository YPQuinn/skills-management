package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSnapshotCoversCompleteSkillTree(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "alpha")
	writeSkill(t, skillDir, "Alpha", "one")
	// supporting content far below the discovery depth is part of the Skill
	deep := filepath.Join(skillDir, "references", "deep", "nested", "file.md")
	if err := os.MkdirAll(filepath.Dir(deep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deep, []byte("supporting"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "assets", "icon.png"), []byte("icon"), 0o755); err != nil {
		t.Fatal(err)
	}

	first, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !first.equal(second) {
		t.Fatal("stable tree must produce equal snapshots")
	}
	paths := map[string]bool{}
	for _, e := range first.entries {
		paths[e.relPath] = true
	}
	for _, want := range []string{
		"skills/alpha",
		"skills/alpha/SKILL.md",
		"skills/alpha/assets/icon.png",
		"skills/alpha/references/deep/nested/file.md",
	} {
		if !paths[want] {
			t.Fatalf("snapshot missing %q (have %v)", want, first.entries)
		}
	}

	// a change to a deeply nested supporting file differs the snapshot
	if err := os.WriteFile(deep, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if first.equal(third) {
		t.Fatal("a supporting-file change must differ the snapshot")
	}
}

func TestSnapshotRootSkillCoversWholeTree(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "Root", "at root")
	deep := filepath.Join(root, "scripts", "deep", "run.sh")
	if err := os.MkdirAll(filepath.Dir(deep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deep, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	first, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, e := range first.entries {
		paths[e.relPath] = true
	}
	for _, want := range []string{".", "SKILL.md", "scripts/deep/run.sh"} {
		if !paths[want] {
			t.Fatalf("root skill snapshot missing %q: %v", want, first.entries)
		}
	}
	// the root Skill directory entry records its executable bit
	for _, e := range first.entries {
		if e.relPath == "." && !e.exec {
			t.Fatalf("root skill directory exec bit not recorded: %+v", e)
		}
	}

	// a supporting change inside the root Skill differs the snapshot
	if err := os.WriteFile(deep, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if first.equal(second) {
		t.Fatal("a root-Skill supporting change must differ the snapshot")
	}
}

func TestSnapshotSkipsOnlyDotGit(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "alpha")
	writeSkill(t, skillDir, "Alpha", "one")
	if err := os.MkdirAll(filepath.Join(skillDir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "node_modules", "pkg", "index.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	base, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	// .git metadata inside a Skill is excluded from the snapshot
	if err := os.MkdirAll(filepath.Join(skillDir, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".git", "objects", "churn"), []byte("changing"), 0o644); err != nil {
		t.Fatal(err)
	}
	withGit, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !base.equal(withGit) {
		t.Fatalf(".git churn must not differ the snapshot: %v vs %v", base.entries, withGit.entries)
	}
	for _, e := range withGit.entries {
		if e.relPath == "skills/alpha/.git" || strings.HasPrefix(e.relPath, "skills/alpha/.git/") {
			t.Fatalf(".git entry leaked into the snapshot: %v", e)
		}
	}

	// node_modules content inside a Skill is part of the tree and differs
	if err := os.WriteFile(filepath.Join(skillDir, "node_modules", "pkg", "index.js"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	withDepChange, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if base.equal(withDepChange) {
		t.Fatal("node_modules content inside a Skill must differ the snapshot")
	}
}

func TestSnapshotRecordsSymlinksAndSpecialNodes(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "alpha")
	writeSkill(t, skillDir, "Alpha", "one")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "target.md"), []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a symlink is recorded as a symlink, never followed or read
	if err := os.Symlink(filepath.Join(outside, "target.md"), filepath.Join(skillDir, "link.md")); err != nil {
		t.Fatal(err)
	}
	// a FIFO is recorded by type and must never be opened
	fifo := filepath.Join(skillDir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}

	first, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]snapshotNodeKind{}
	var linkTarget string
	for _, e := range first.entries {
		kinds[e.relPath] = e.kind
		if e.relPath == "skills/alpha/link.md" {
			linkTarget = e.linkTarget
		}
	}
	if kinds["skills/alpha/link.md"] != kindSymlink || linkTarget == "" {
		t.Fatalf("symlink not recorded as a symlink: %+v", first.entries)
	}
	if kinds["skills/alpha/pipe"] != kindFIFO {
		t.Fatalf("FIFO not recorded by type: %+v", first.entries)
	}

	// changing the symlink target differs the snapshot without opening it
	if err := os.Remove(filepath.Join(skillDir, "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "other.md"), filepath.Join(skillDir, "link.md")); err != nil {
		t.Fatal(err)
	}
	second, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if first.equal(second) {
		t.Fatal("a symlink-target change must differ the snapshot")
	}
}

func TestSnapshotDigestDeterministicAndContentBound(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	deep := filepath.Join(root, "skills", "alpha", "references", "a.md")
	if err := os.MkdirAll(filepath.Dir(deep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deep, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	d1 := snapshotDigest(snap, "skills/alpha")
	d2 := snapshotDigest(snap, "skills/alpha")
	if d1 != d2 {
		t.Fatalf("digest must be deterministic: %q vs %q", d1, d2)
	}

	// a supporting-file change rebinds the entry digest
	if err := os.WriteFile(deep, []byte("other content"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap2, err := snapshotRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if d3 := snapshotDigest(snap2, "skills/alpha"); d3 == d1 {
		t.Fatal("a supporting-file change must change the entry digest")
	}
}
