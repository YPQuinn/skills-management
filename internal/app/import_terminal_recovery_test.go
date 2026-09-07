package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// reopenApp closes a and returns a fresh App bound to the same Store and
// state database, simulating a process restart at the exact crash window.
// The fresh App runs open-time recovery, exactly like production New.
func reopenApp(t *testing.T, a *App) *App {
	t.Helper()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	fresh, err := New(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fresh.Close() })
	return fresh
}

// reopenAppUnrecovered closes a and returns a fresh App bound to the same
// Store and state database WITHOUT open-time recovery. Tests use it when
// the recovery itself is the subject: to install a seam before the lazy
// recovery inside the next Store write runs, or to observe the failure of
// a recovery that cannot be proven (the production App refuses such an
// open with recovery_failed).
func reopenAppUnrecovered(t *testing.T, a *App) *App {
	t.Helper()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	fresh, err := openApp(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fresh.Close() })
	return fresh
}

// importAndStopAfterReceiptPersisted builds a committed import, runs the
// semantic finalization, and persists the terminal receipt, leaving the
// receipt-bound evidence (opDir and proof) for the fresh process to clean.
func importAndStopAfterReceiptPersisted(t *testing.T, a *App, src *source.Source, relDir, slug string) skillstore.Operation {
	t.Helper()
	op := importAndStopBeforeFinalize(t, a, src, relDir, slug)
	receipt, err := a.store.PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	op.Phase = skillstore.PhaseFinalized
	op.Receipt = receipt.Bytes()
	if err := state.MarkOperationFinalized(a.db, op); err != nil {
		t.Fatal(err)
	}
	return op
}

// TestImportSkillsFreshAppBlocksForeignLiveSwappedAtEvidenceRemoval proves
// the post-cleanup terminal validation at the application seam: a
// byte-identical foreign live tree substituted at the final evidence-
// removal hook is refused with CodeRecovery, the terminal row and its
// receipt are kept, and the foreign object is preserved.
func TestImportSkillsFreshAppBlocksForeignLiveSwappedAtEvidenceRemoval(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopAfterReceiptPersisted(t, a, src, "skills/alpha", "alpha")

	// the recovery runs in a genuinely fresh App, with the substitution
	// hook installed on the fresh Store (open without open-time recovery
	// so the hook is in place when the next Store write recovers)
	fresh := reopenAppUnrecovered(t, a)
	fresh.store.SetHook(func(p skillstore.HookPoint) {
		if p != skillstore.HookAfterRemoveOp {
			return
		}
		live := filepath.Join(a.StorePath, "alpha")
		real := live + ".real"
		if err := os.Rename(live, real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyDirForTest(swap, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, live); err != nil {
			t.Fatal(err)
		}
	})
	defer fresh.store.SetHook(nil)

	_, err := fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("foreign live at the evidence-removal boundary: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseFinalized {
		t.Fatalf("the terminal row must be kept with its receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil {
		t.Fatal("the byte-identical foreign live tree must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha.real", "SKILL.md")); err != nil {
		t.Fatal("the installed live tree must survive beside the foreign one")
	}
}

// TestImportSkillsFreshAppBlocksPartialQuarantineDrain proves the
// mid-drain crash state at the application boundary: a restore whose
// quarantine drain was interrupted (content removed, a foreign child
// injected after the crash) cannot be certified by the digest gate, so
// recovery fails with CodeRecovery, the row stays pending, and every
// candidate including the injected child is preserved.
func TestImportSkillsFreshAppBlocksPartialQuarantineDrain(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeCommit(t, a, src, "skills/alpha", "alpha")
	// reproduce the interrupted quarantine drain: the live tree sits in
	// the quarantine with its content removed and a foreign child inserted
	q := filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "live-quarantine")
	if err := os.Rename(filepath.Join(a.StorePath, "alpha"), q); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(q, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(q, "injected.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The production open refuses the unprovable recovery before exposing
	// any capability.
	if _, err := New(a.StorePath, a.StateDBPath); !isCode(err, CodeRecovery) {
		t.Fatalf("open must refuse an unprovable recovery: %v", err)
	}

	fresh := reopenAppUnrecovered(t, a)
	_, err := fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("partial quarantine drain: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("the intent must stay pending: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(q, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected foreign child must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
}

// TestImportSkillsFreshAppConvergesAfterReceiptPersistedBeforeCleanup proves
// the receipt-persisted-before-cleanup crash window at the application
// boundary: the semantic finalize and the durable receipt CAS completed,
// the evidence cleanup did not run. A fresh App validates the receipt-bound
// terminal result, removes the exact evidence, and CAS-deletes the row; the
// already-committed entry then reports already_imported.
func TestImportSkillsFreshAppConvergesAfterReceiptPersistedBeforeCleanup(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopAfterReceiptPersisted(t, a, src, "skills/alpha", "alpha")
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("test setup must leave the evidence for the fresh process")
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != op.SkillID {
		t.Fatalf("item after terminal convergence: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the terminal row must be cleared: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("the fresh process must remove the receipt-bound evidence")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(op.SkillID), "SKILL.md")); err != nil {
		t.Fatal("the installed Baseline must survive")
	}
}

// TestImportSkillsFreshAppResumesPartialTerminalCleanup proves the
// partially-completed cleanup crash window: the proof was already removed
// when the process died. The fresh App resumes the cleanup by the
// receipt-bound operation-directory identity and converges.
func TestImportSkillsFreshAppResumesPartialTerminalCleanup(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopAfterReceiptPersisted(t, a, src, "skills/alpha", "alpha")
	if err := os.Remove(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal(err)
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported {
		t.Fatalf("item after resumed cleanup: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the terminal row must be cleared: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("the empty operation directory must be removed on resume")
	}
}

// TestImportSkillsFreshAppBlocksForeignLiveOnTerminalRow proves a
// byte-identical foreign live tree substituted after the receipt persisted
// is refused by the receipt-bound identity: the fresh App preserves the
// foreign tree and the evidence, keeps the terminal row, and blocks with
// CodeRecovery.
func TestImportSkillsFreshAppBlocksForeignLiveOnTerminalRow(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopAfterReceiptPersisted(t, a, src, "skills/alpha", "alpha")
	live := filepath.Join(a.StorePath, "alpha")
	real := live + ".real"
	if err := os.Rename(live, real); err != nil {
		t.Fatal(err)
	}
	swap := t.TempDir()
	if err := copyDirForTest(swap, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(swap, live); err != nil {
		t.Fatal(err)
	}

	fresh := reopenAppUnrecovered(t, a)
	_, err := fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("foreign live on a terminal row: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseFinalized {
		t.Fatalf("the terminal row must be kept: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the evidence must be preserved with the foreign live tree")
	}
	if _, err := os.Lstat(filepath.Join(real, "SKILL.md")); err != nil {
		t.Fatal("the installed live tree must survive beside the foreign one")
	}
}

// TestImportSkillsFreshAppBlocksByteIdenticalForeignProof proves the
// durable proof self-identity binding at the application boundary: a
// byte-identical proof copied into a fresh inode after the commit is
// refused by fresh-App recovery with CodeRecovery, the committed row is
// kept, and the foreign proof and operation evidence are preserved.
func TestImportSkillsFreshAppBlocksByteIdenticalForeignProof(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeFinalize(t, a, src, "skills/alpha", "alpha")
	proofPath := filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")
	data, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatal(err)
	}
	tmp := proofPath + ".copy"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, proofPath); err != nil {
		t.Fatal(err)
	}

	fresh := reopenAppUnrecovered(t, a)
	_, err = fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("byte-identical foreign proof: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("the committed row must be kept: %+v", ops)
	}
	if _, err := os.Lstat(proofPath); err != nil {
		t.Fatal("the byte-identical foreign proof must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "baseline")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
}
