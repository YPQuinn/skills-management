package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stageAndInstall stages a materialized tree under the op and installs it.
func stageAndInstall(t *testing.T, s Store, op Operation, m materialized) {
	t.Helper()
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(context.Background(), op, &staged); err != nil {
		t.Fatal(err)
	}
}

func TestInstallImport(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{
		"SKILL.md":     "---\nname: Alpha\ndescription: one\n---\n",
		"tools/run.sh": "#!/bin/sh\n",
	})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)

	live := liveDir(t, s, "alpha")
	if _, err := os.Lstat(filepath.Join(live, "SKILL.md")); err != nil {
		t.Fatalf("live tree missing: %v", err)
	}
	// the Baseline candidate was prepared inside staging
	candidate := s.baselineCandidateDir(1)
	if _, err := os.Lstat(filepath.Join(candidate, "SKILL.md")); err != nil {
		t.Fatalf("baseline candidate missing: %v", err)
	}
	// no recovery slot exists for an import
	if _, err := os.Lstat(s.recoveryDir(1)); !os.IsNotExist(err) {
		t.Fatalf("import must not create a recovery slot, stat: %v", err)
	}
	// the staged tree moved into the live path
	if _, err := os.Lstat(s.stagedTreeDir(1)); !os.IsNotExist(err) {
		t.Fatalf("staged tree must be consumed, stat: %v", err)
	}
}

func TestInstallImportRefusesUnmanagedContent(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}

	// a pre-existing directory that Skill Manager does not manage
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "precious.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Install(context.Background(), op, &staged)
	if !errors.Is(err, ErrUnmanaged) {
		t.Fatalf("unmanaged live content: got %v, want ErrUnmanaged", err)
	}
	// the unmanaged content is untouched
	data, err := os.ReadFile(filepath.Join(live, "precious.txt"))
	if err != nil || string(data) != "mine" {
		t.Fatalf("unmanaged content must be preserved: %q, %v", data, err)
	}
}

// TestInstallImportNoReplaceRace proves the no-replace rename protects a
// live path that appears between the existence check and the install.
func TestInstallImportNoReplaceRace(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	// the live path appears after the existence check, before the rename
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "raced.txt"), []byte("raced"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = s.Install(context.Background(), op, &staged)
	if !errors.Is(err, ErrUnmanaged) {
		t.Fatalf("raced live content: got %v, want ErrUnmanaged", err)
	}
	if data, err := os.ReadFile(filepath.Join(live, "raced.txt")); err != nil || string(data) != "raced" {
		t.Fatalf("raced content must be preserved: %q, %v", data, err)
	}
	// the staged tree still exists for the caller's cleanup/recovery
	if _, err := os.Lstat(s.stagedTreeDir(1)); err != nil {
		t.Fatalf("staged tree must survive a refused install: %v", err)
	}
}

func TestInstallReplaceMovesOldToRecovery(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	first := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: old.digest}
	stageAndInstall(t, s, first, old)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)

	live := liveDir(t, s, "alpha")
	data, err := os.ReadFile(filepath.Join(live, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new content" {
		t.Fatalf("live content after replace: %q", data)
	}
	// the old content is preserved in the recovery slot
	rec, err := os.ReadFile(filepath.Join(s.recoveryDir(2), "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rec) != "old content" {
		t.Fatalf("recovery content: %q", rec)
	}
	// the Baseline candidate holds the new content
	cand, err := os.ReadFile(filepath.Join(s.baselineCandidateDir(2), "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cand) != "new content" {
		t.Fatalf("baseline candidate: %q", cand)
	}
}

// TestInstallReplaceRejectsChangedLive proves a replace never overwrites a
// live tree that changed since the operation was prepared.
func TestInstallReplaceRejectsChangedLive(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)
	// the live tree changes after the replace was prepared
	if err := os.WriteFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	staged, err := s.Stage(context.Background(), op.ID, replacement.dir, replacement.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Install(context.Background(), op, &staged)
	if err == nil || !strings.Contains(err.Error(), "changed since the replace was prepared") {
		t.Fatalf("changed live: got %v, want a stale-replace error", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); readErr != nil || string(data) != "edited" {
		t.Fatalf("edited live content must be preserved: %q, %v", data, readErr)
	}
}

// TestInstallRejectsStagedDigestMismatch proves Install re-proves the
// staged tree before any live mutation.
func TestInstallRejectsStagedDigestMismatch(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.stagedTreeDir(op.ID), "SKILL.md"), []byte("swapped"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = s.Install(context.Background(), op, &staged)
	if err == nil || !strings.Contains(err.Error(), "does not match the expected digest") {
		t.Fatalf("staged mismatch: got %v, want a digest error", err)
	}
	if _, statErr := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(statErr) {
		t.Fatalf("no live tree may be installed from a mismatched staging, stat: %v", statErr)
	}
}

// TestInstallRejectsUnsafeSlug proves a slug can never escape the Store
// through the operation journal.
func TestInstallRejectsUnsafeSlug(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "../escape", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(context.Background(), op, &staged); err == nil {
		t.Fatal("an unsafe slug must be rejected")
	}
	if _, statErr := os.Lstat(filepath.Join(s.Root, "..", "escape")); !os.IsNotExist(statErr) {
		t.Fatalf("the slug must not escape the Store, stat: %v", statErr)
	}
}

func TestInstallReplaceMissingLive(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindReplace, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Install(context.Background(), op, &staged)
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("missing live tree: got %v, want ErrMissing", err)
	}
}

// TestInstallReplaceDisplacesNonDirectoryLive locks the unreadable replace
// path: a replace whose OldDigest is empty (an Accept Source over a plain
// file) displaces the non-directory live node by physical identity only and
// installs the staged tree.
func TestInstallReplaceDisplacesNonDirectoryLive(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(liveDir(t, s, "alpha"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	op := Operation{ID: 1, Slug: "alpha", Kind: KindReplace, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(context.Background(), op, &staged); err != nil {
		t.Fatalf("unreadable replace: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(s.Root, "alpha", "SKILL.md")); err != nil || string(data) != "content" {
		t.Fatalf("installed content: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(s.Root, ".skillctl", "recovery", "1")); err != nil || string(data) != "file" {
		t.Fatalf("recovery slot must hold the displaced node: %q, %v", data, err)
	}
}

func TestInstallRejectsUnknownKind(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: "rebind", NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(context.Background(), op, &staged); err == nil {
		t.Fatal("unknown operation kind: want error")
	}
}
