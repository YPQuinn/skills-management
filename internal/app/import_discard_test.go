package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
)

// TestImportDiscardMissingOpDirKeepsPending proves a pre-install refusal
// whose discard finds the expected operation directory missing is ambiguous:
// the app never deletes the SQL journal, the intent stays pending, and the
// unmanaged live content is preserved.
func TestImportDiscardMissingOpDirKeepsPending(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	// unmanaged content occupies the alpha slug, so Install refuses with
	// ErrUnmanaged and the app discards the staging with the opaque proof
	unmanaged := filepath.Join(a.StorePath, "alpha")
	if err := os.MkdirAll(unmanaged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unmanaged, "mine.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	n := 0
	a.store.SetHook(func(p skillstore.HookPoint) {
		// Install fires the layout hook first; the discard fires it second
		if p != skillstore.HookAfterLayoutOpen {
			return
		}
		n++
		if n != 2 {
			return
		}
		staging := filepath.Join(a.StorePath, ".skillctl", "staging")
		entries, err := os.ReadDir(staging)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected one operation directory, got %d", len(entries))
		}
		if err := os.RemoveAll(filepath.Join(staging, entries[0].Name())); err != nil {
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
		t.Fatalf("ambiguous discard must block with CodeRecovery: %+v", item)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("the operation must stay pending, never deleted: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(unmanaged, "mine.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("unmanaged content must be preserved: %q, %v", data, rerr)
	}
}

// TestImportSkillsCommittedJournalRetainedOnFinalizeRefusal proves a
// committed operation whose finalization cannot complete (a foreign child
// blocks the strict operation cleanup) leaves the committed intent in the
// SQL journal: the app reports CodeRecovery and never deletes the row.
func TestImportSkillsCommittedJournalRetainedOnFinalizeRefusal(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeFinalize(t, a, src, "skills/alpha", "alpha")
	if err := os.WriteFile(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "foreign.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("blocked committed finalization: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseFinalized || len(ops[0].Receipt) == 0 {
		t.Fatalf("the terminal finalized intent must be retained with its receipt: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "foreign.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the foreign child must be preserved: %q, %v", data, rerr)
	}
}
