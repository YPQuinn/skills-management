package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findTrashEntry returns the name of the single .skill-del-* trash entry
// under dir, failing the test if it is absent.
func findTrashEntry(t *testing.T, dir string) string {
	t.Helper()
	name := findTrashEntryOrEmpty(t, dir)
	if name == "" {
		t.Fatalf("no trash entry under %q", dir)
	}
	return name
}

// findTrashEntryOrEmpty returns the .skill-del-* entry under dir, or ""
// when none exists.
func findTrashEntryOrEmpty(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".skill-del-") {
			return e.Name()
		}
	}
	return ""
}

// findTerminalSlotOrEmpty returns the .skillctl-term-* terminal slot under
// dir, or "" when none exists.
func findTerminalSlotOrEmpty(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), terminalSlotPrefix) {
			return e.Name()
		}
	}
	return ""
}

// TestFinalizeProofSwapAfterDetachKeepsForeign proves the strict removeOp
// proof deletion: after the proof was detached into its trash slot, a
// foreign proof file swapped into the now-empty original logical name is
// never unlinked — the owned proof is deleted from the trash slot, the
// foreign file stays inside the operation directory, and Finalize reports
// ErrAmbiguous with the journal retained.
func TestFinalizeProofSwapAfterDetachKeepsForeign(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	foreign := t.TempDir()
	if err := os.WriteFile(filepath.Join(foreign, "proof"), []byte("foreign proof"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { testBeforeDeleteUnlinkHook = nil }()
	testBeforeDeleteUnlinkHook = func(parent *os.Root, name, trashName string) {
		if name != "proof" {
			return
		}
		// the original logical name is already empty after the detach; a
		// foreign proof file appears there and must never be unlinked
		if err := os.Rename(filepath.Join(foreign, "proof"), filepath.Join(s.opDir(op.ID), "proof")); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign proof after the detach: got %v, want ErrAmbiguous", err)
	}
	staging := filepath.Join(s.InternalDir(), "staging")
	trash := findTerminalSlotOrEmpty(t, staging)
	if trash == "" {
		t.Fatal("the refused cleanup must preserve the operation directory in its terminal slot")
	}
	// the foreign proof survives inside the operation directory preserved
	// in its terminal slot; the owned proof was deleted from its own slot
	if data, rerr := os.ReadFile(filepath.Join(staging, trash, "proof")); rerr != nil || string(data) != "foreign proof" {
		t.Fatalf("the foreign proof must be preserved: %q, %v", data, rerr)
	}
	entries, err := os.ReadDir(filepath.Join(staging, trash))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "proof" {
		t.Fatalf("only the foreign proof may remain in the preserved operation directory: %v", entries)
	}
	// the proven phases completed before the cleanup refused
	if data, rerr := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); rerr != nil || string(data) != "content" {
		t.Fatalf("the Baseline must be installed before the cleanup refused: %q, %v", data, rerr)
	}
}

// TestRestoreImportQuarantineDirSwapAfterDetachKeepsForeign proves the
// directory-transient drain: after the quarantined live tree was detached,
// a foreign directory swapped into the now-empty original logical name is
// never unlinked — the owned quarantine is deleted from its trash slot,
// the foreign directory stays at the original name, and the restore
// reports ErrAmbiguous with the operation evidence preserved.
func TestRestoreImportQuarantineDirSwapAfterDetachKeepsForeign(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	foreign := t.TempDir()
	if err := os.WriteFile(filepath.Join(foreign, "foreign.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { testBeforeDeleteUnlinkHook = nil }()
	testBeforeDeleteUnlinkHook = func(parent *os.Root, name, trashName string) {
		if name != quarantineName {
			return
		}
		// the original logical name is already empty after the detach; a
		// foreign directory appears there and must never be unlinked
		if err := os.Rename(foreign, filepath.Join(s.opDir(op.ID), quarantineName)); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign quarantine after the detach: got %v, want ErrAmbiguous", err)
	}
	// the foreign directory stays at the original logical name
	if data, rerr := os.ReadFile(filepath.Join(s.opDir(op.ID), quarantineName, "foreign.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the foreign directory must stay at the original name: %q, %v", data, rerr)
	}
	// the owned quarantine was deleted from its trash slot
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), ".skill-del-2")); !os.IsNotExist(err) {
		t.Fatal("the owned quarantine trash slot must be consumed")
	}
	// the operation journal is preserved
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
}

// TestRestoreImportQuarantineFileSwapAfterDetachKeepsForeign proves the
// regular-file transient drain: after a file inside the quarantined tree
// was detached, a foreign file swapped into its now-empty original logical
// name is never unlinked — the owned file is deleted from its trash slot,
// the foreign file stays inside the quarantine, and the restore reports
// ErrAmbiguous with the quarantine and the operation evidence preserved.
func TestRestoreImportQuarantineFileSwapAfterDetachKeepsForeign(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	foreign := t.TempDir()
	if err := os.WriteFile(filepath.Join(foreign, "SKILL.md"), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { testBeforeDeleteUnlinkHook = nil }()
	testBeforeDeleteUnlinkHook = func(parent *os.Root, name, trashName string) {
		if name != "SKILL.md" {
			return
		}
		// the original logical name is already empty after the detach; a
		// foreign file appears there and must never be unlinked
		q := filepath.Join(s.opDir(op.ID), quarantineName)
		if err := os.Rename(filepath.Join(foreign, "SKILL.md"), filepath.Join(q, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign file after the detach: got %v, want ErrAmbiguous", err)
	}
	opDir := s.opDir(op.ID)
	trash := findTrashEntry(t, opDir)
	// the foreign file is preserved inside the quarantine preserved in its
	// trash slot
	if data, rerr := os.ReadFile(filepath.Join(opDir, trash, "SKILL.md")); rerr != nil || string(data) != "foreign" {
		t.Fatalf("the foreign file must be preserved: %q, %v", data, rerr)
	}
	// the operation journal is preserved
	if _, err := os.Lstat(filepath.Join(opDir, "proof")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
}

// TestDiscardStagingFileSwapAfterDetachKeepsForeign proves the ownership
// cleanup primitive: after a staged file was detached, a foreign file
// swapped into its now-empty original logical name is never unlinked — the
// owned file is deleted from its trash slot, the foreign file stays inside
// the staged tree, and the discard reports ErrAmbiguous with the operation
// directory preserved.
func TestDiscardStagingFileSwapAfterDetachKeepsForeign(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	foreign := t.TempDir()
	if err := os.WriteFile(filepath.Join(foreign, "SKILL.md"), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { testBeforeDeleteUnlinkHook = nil }()
	testBeforeDeleteUnlinkHook = func(parent *os.Root, name, trashName string) {
		if name != "SKILL.md" {
			return
		}
		// the original logical name is already empty after the detach; a
		// foreign file appears there and must never be unlinked
		tree := s.stagedTreeDir(op.ID)
		if err := os.Rename(filepath.Join(foreign, "SKILL.md"), filepath.Join(tree, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}

	err = s.DiscardStaging(op.ID, staged)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign file after the detach: got %v, want ErrAmbiguous", err)
	}
	// the foreign file stays at its original logical name inside the staged
	// tree, which the empty-directory pre-check refused to detach
	tree := s.stagedTreeDir(op.ID)
	if data, rerr := os.ReadFile(filepath.Join(tree, "SKILL.md")); rerr != nil || string(data) != "foreign" {
		t.Fatalf("the foreign file must stay at its original name: %q, %v", data, rerr)
	}
	// the owned file was deleted from its own trash slot: no trash entry
	// remains inside the staged tree
	if trash := findTrashEntryOrEmpty(t, tree); trash != "" {
		t.Fatalf("the owned file trash slot must be consumed, left %q", trash)
	}
	// the operation directory is preserved
	if _, err := os.Lstat(s.opDir(op.ID)); err != nil {
		t.Fatal("the operation directory must be preserved")
	}
}
