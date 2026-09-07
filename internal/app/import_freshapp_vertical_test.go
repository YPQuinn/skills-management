package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
)

const v2Markdown = "---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n"
const v1Markdown = "---\nname: Alpha\ndescription: desc\n---\n# Alpha\n"

// addReplacementSource registers a second Local Source whose alpha entry
// carries the v2 content.
func addReplacementSource(t *testing.T, a *App) *source.Source {
	t.Helper()
	rootB := t.TempDir()
	writeSkillFile(t, rootB, "skills/alpha", "Alpha")
	if err := os.WriteFile(filepath.Join(rootB, "skills", "alpha", "SKILL.md"), []byte(v2Markdown), 0o644); err != nil {
		t.Fatal(err)
	}
	srcB, err := a.AddSource(context.Background(), sourceAddInput(rootB))
	if err != nil {
		t.Fatal(err)
	}
	return srcB
}

// TestImportSkillsReplaceFinalizeReceiptCASFailureConvergesOnFreshApp
// proves the replace finalize crash window through a real failed CAS: the
// semantic finalization completed, the durable receipt CAS failed, and the
// row stayed committed. A fresh App re-prepares the same terminal state,
// persists the receipt, cleans the evidence, and clears the row with live,
// Baseline, and previous in their terminal state.
func TestImportSkillsReplaceFinalizeReceiptCASFailureConvergesOnFreshApp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", first.Items[0])
	}
	srcB := addReplacementSource(t, a)
	op := replaceAndCommit(t, a, srcB, "skills/alpha", "alpha")
	a.receiptPersist = casFailure()

	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !isCode(err, CodeRecovery) {
		t.Fatalf("failed receipt CAS must block with CodeRecovery: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseCommitted || len(ops[0].Receipt) != 0 {
		t.Fatalf("the replace row must stay committed without a receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the evidence must be retained after the CAS failure")
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, srcB.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != op.SkillID {
		t.Fatalf("item after fresh-App convergence: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the replace row must be finalized and cleared: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); rerr != nil || string(data) != v2Markdown {
		t.Fatalf("the replaced live tree: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(op.SkillID), "SKILL.md")); rerr != nil || string(data) != v2Markdown {
		t.Fatalf("the advanced Baseline: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "previous", itoa(op.SkillID), "SKILL.md")); rerr != nil || string(data) != v1Markdown {
		t.Fatalf("the rotated previous snapshot: %q, %v", data, rerr)
	}
}

// TestImportSkillsRestoreTerminalDeleteCASFailureConvergesOnFreshApp proves
// the restore terminal-delete crash window with a fresh process: the
// receipt persisted and the evidence was cleaned, the exact receipt CAS
// deletion failed, and the row stayed restored. A fresh App validates the
// already-clean receipt-bound restored result, CAS-deletes the row, and
// imports the entry fresh.
func TestImportSkillsRestoreTerminalDeleteCASFailureConvergesOnFreshApp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			cancel()
		}
	}
	a.deleteOperation = func(int64) error { return errors.New("delete failed") }

	res, err := a.ImportSkills(ctx, ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Status != StatusFailed || res.Items[0].ErrorCode != CodeRecovery {
		t.Fatalf("restored-but-uncleared item: %+v", res.Items[0])
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseRestored || len(ops[0].Receipt) == 0 {
		t.Fatalf("the residual restored row with its receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(ops[0].ID))); !os.IsNotExist(err) {
		t.Fatal("the receipt-bound evidence must be cleaned before the delete")
	}

	fresh := reopenApp(t, a)
	again := importSkills(t, fresh, src.ID, "skills/alpha")
	if again.Items[0].Status != StatusImported {
		t.Fatalf("item after fresh-App convergence: %+v", again.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the restored row must be CAS-deleted by the fresh App: %+v", ops)
	}
}

// TestImportSkillsReplaceFinalizeDeleteCASFailureConvergesOnFreshApp proves
// the replace finalize terminal-delete crash window with a fresh process:
// the replace terminal state (live, Baseline, previous) is validated and
// the row is CAS-deleted.
func TestImportSkillsReplaceFinalizeDeleteCASFailureConvergesOnFreshApp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", first.Items[0])
	}
	srcB := addReplacementSource(t, a)
	op := replaceAndStopAfterReceiptPersisted(t, a, srcB, "skills/alpha", "alpha")
	a.deleteOperation = func(int64) error { return errors.New("delete failed") }

	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !isCode(err, CodeRecovery) {
		t.Fatalf("failed terminal delete must block with CodeRecovery: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseFinalized || len(ops[0].Receipt) == 0 {
		t.Fatalf("the replace row must stay finalized with its receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("the receipt-bound evidence must be cleaned before the delete")
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, srcB.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != op.SkillID {
		t.Fatalf("item after fresh-App convergence: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the replace terminal row must be CAS-deleted: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "previous", itoa(op.SkillID), "SKILL.md")); rerr != nil || string(data) != v1Markdown {
		t.Fatalf("the rotated previous snapshot must survive: %q, %v", data, rerr)
	}
}
