package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
)

// copyTreeForApp duplicates one directory tree into another for test state
// construction.
func copyTreeForApp(dst, src string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// TestImportStageForeignSwapKeepsPending proves the application never
// aborts an operation whose staging became ambiguous: a byte-identical
// foreign operation directory swapped in after the staged tree was hashed
// makes Stage fail with ErrAmbiguous, the item reports CodeRecovery, the
// operation intent stays pending, and the foreign operation directory is
// preserved for the next recovery.
func TestImportStageForeignSwapKeepsPending(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	a.store.SetHook(func(p skillstore.HookPoint) {
		if p != skillstore.HookAfterStageHash {
			return
		}
		staging := filepath.Join(a.StorePath, ".skillctl", "staging")
		entries, err := os.ReadDir(staging)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected exactly one operation directory, got %d", len(entries))
		}
		opDir := filepath.Join(staging, entries[0].Name())
		swap := t.TempDir()
		if err := copyTreeForApp(swap, opDir); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(opDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, opDir); err != nil {
			t.Fatal(err)
		}
	})
	defer a.store.SetHook(nil)

	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := res.Items[0]
	if item.Status != StatusFailed || item.ErrorCode != CodeRecovery {
		t.Fatalf("ambiguous staging must block with CodeRecovery: %+v", item)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("the operation must stay pending, never aborted: %+v", ops)
	}
	entries, err := os.ReadDir(filepath.Join(a.StorePath, ".skillctl", "staging"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("the foreign operation directory must survive: %v, %d entries", err, len(entries))
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); !os.IsNotExist(err) {
		t.Fatal("no live tree may be installed from swapped staging")
	}
}

// TestImportRecoveryStagedOnlyPreserves proves restart recovery of a
// pending intent with surviving staging preserves the intent, the staging,
// and the live tree: without a persisted physical proof (Stage-A writes no
// durable receipt) recovery reports CodeRecovery and never drains a fresh
// sample of the operation name.
func TestImportRecoveryStagedOnlyPreserves(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopAfterStage(t, a, src, "skills/alpha", "alpha")

	_, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("staged-only recovery must block: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("the intent must stay pending: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); err != nil {
		t.Fatal("the staging must be preserved")
	}
}
