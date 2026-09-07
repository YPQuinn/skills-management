package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
)

// casFailure returns a receiptPersist seam that fails once, simulating a
// crash between the Store terminal preparation and the durable receipt CAS.
func casFailure() func(skillstore.Operation) error {
	return func(skillstore.Operation) error { return errors.New("receipt CAS failed") }
}

// TestImportSkillsFinalizeReceiptCASFailureConvergesOnFreshApp proves the
// finalize crash window through a real failed CAS at the application
// boundary: the Store semantic finalization completed, the durable receipt
// CAS failed, and the row stayed committed without a receipt. A fresh App
// re-prepares the same terminal state, persists the receipt, cleans the
// evidence, and clears the row.
func TestImportSkillsFinalizeReceiptCASFailureConvergesOnFreshApp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeFinalize(t, a, src, "skills/alpha", "alpha")
	a.receiptPersist = casFailure()

	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !isCode(err, CodeRecovery) {
		t.Fatalf("failed receipt CAS must block with CodeRecovery: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseCommitted || len(ops[0].Receipt) != 0 {
		t.Fatalf("the row must stay committed without a receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the evidence must be retained after the CAS failure")
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != op.SkillID {
		t.Fatalf("item after fresh-App convergence: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the committed row must be finalized and cleared: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(op.SkillID), "SKILL.md")); err != nil {
		t.Fatalf("the fresh App must finalize the Baseline: %v", err)
	}
}

// TestImportSkillsRestoreReceiptCASFailureConvergesOnFreshApp proves the
// import restore crash window through a real failed CAS: the semantic
// restore removed the installed live tree, the receipt CAS failed, and the
// row stayed pending. A fresh App re-prepares the already-restored state,
// persists the receipt, cleans the evidence, and clears the row, and the
// entry imports fresh.
func TestImportSkillsRestoreReceiptCASFailureConvergesOnFreshApp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeCommit(t, a, src, "skills/alpha", "alpha")
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); err != nil {
		t.Fatal("test setup must install the live tree")
	}
	a.receiptPersist = casFailure()

	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !isCode(err, CodeRecovery) {
		t.Fatalf("failed receipt CAS must block with CodeRecovery: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending || len(ops[0].Receipt) != 0 {
		t.Fatalf("the row must stay pending without a receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); !os.IsNotExist(err) {
		t.Fatal("the semantic restore must have removed the uncommitted live tree")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the evidence must be retained after the CAS failure")
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusImported {
		t.Fatalf("item after fresh-App convergence: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the restored row must be cleared: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("the fresh App must clean the receipt-bound evidence")
	}
}

// TestImportSkillsReplaceRestoreReceiptCASFailureConvergesOnFreshApp proves
// the replace restore crash window through a real failed CAS: the semantic
// restore moved the recovery slot back to live, the receipt CAS failed,
// and the row stayed pending. A fresh App resumes only by the live tree's
// physical identity matching the proof's recovery identity, persists the
// receipt, cleans the evidence, and clears the row.
func TestImportSkillsReplaceRestoreReceiptCASFailureConvergesOnFreshApp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	skillID := first.Items[0].SkillID
	rootB := t.TempDir()
	writeSkillFile(t, rootB, "skills/alpha", "Alpha")
	if err := os.WriteFile(filepath.Join(rootB, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcB, err := a.AddSource(context.Background(), sourceAddInput(rootB))
	if err != nil {
		t.Fatal(err)
	}
	dir, digest := materializeTo(t, a, srcB, "skills/alpha")
	detail, err := state.GetSkillDetailBySlug(a.db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	op := skillstore.Operation{
		SkillID: detail.Skill.ID, Slug: detail.Skill.Slug, Kind: skillstore.KindReplace,
		OldDigest: detail.Skill.StoreDigest, NewDigest: digest,
	}
	op, err = state.InsertOperation(a.db, op)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := a.store.Stage(context.Background(), op.ID, dir, digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.Install(context.Background(), op, &staged); err != nil {
		t.Fatal(err)
	}
	a.receiptPersist = casFailure()

	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !isCode(err, CodeRecovery) {
		t.Fatalf("failed receipt CAS must block with CodeRecovery: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending || len(ops[0].Receipt) != 0 {
		t.Fatalf("the replace row must stay pending without a receipt: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); rerr != nil ||
		string(data) == "---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n" {
		t.Fatalf("the semantic restore must have restored the old content: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the evidence must be retained after the CAS failure")
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != skillID {
		t.Fatalf("item after fresh-App convergence: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the replace restore row must be cleared: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); rerr != nil ||
		string(data) == "---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n" {
		t.Fatalf("the restored old content must be live: %q, %v", data, rerr)
	}
}

// TestImportSkillsTerminalDeleteCASFailureConvergesOnFreshApp proves the
// terminal delete crash window with a real failed CAS and a fresh process:
// the receipt persisted and the evidence was cleaned, the exact receipt CAS
// deletion failed, and the row stayed finalized with its receipt. A fresh
// App validates the already-clean receipt-bound terminal result,
// CAS-deletes the row, and converges.
func TestImportSkillsTerminalDeleteCASFailureConvergesOnFreshApp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopAfterReceiptPersisted(t, a, src, "skills/alpha", "alpha")
	a.deleteOperation = func(int64) error { return errors.New("delete failed") }

	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !isCode(err, CodeRecovery) {
		t.Fatalf("failed terminal delete must block with CodeRecovery: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseFinalized || len(ops[0].Receipt) == 0 {
		t.Fatalf("the row must stay finalized with its receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("the receipt-bound evidence must be cleaned before the delete")
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != op.SkillID {
		t.Fatalf("item after fresh-App convergence: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the terminal row must be CAS-deleted by the fresh App: %+v", ops)
	}
}
