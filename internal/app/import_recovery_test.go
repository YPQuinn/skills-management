package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
)

// TestImportSkillsRecoversPendingAfterStage proves lazy recovery of a
// pending intent whose staging was never installed: a surviving staged tree
// has no persisted physical proof on restart (Stage-A writes no durable
// receipt), so recovery preserves the staging and the intent with
// CodeRecovery instead of draining a fresh sample of the operation name.
func TestImportSkillsRecoversPendingAfterStage(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopAfterStage(t, a, src, "skills/alpha", "alpha")
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); !os.IsNotExist(err) {
		t.Fatal("test setup must leave no live tree")
	}

	_, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("staged-only recovery must block: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("intent must stay pending: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); err != nil {
		t.Fatal("staging must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); !os.IsNotExist(err) {
		t.Fatal("no live tree may be installed")
	}
}

// TestImportSkillsRecoversPendingAfterInstall proves lazy recovery removes
// an installed-but-uncommitted live tree (proven by the consumed staged
// tree plus the Baseline candidate) before the fresh import proceeds.
func TestImportSkillsRecoversPendingAfterInstall(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeCommit(t, a, src, "skills/alpha", "alpha")
	live := filepath.Join(a.StorePath, "alpha")
	if _, err := os.Lstat(live); err != nil {
		t.Fatal("test setup must install the live tree")
	}

	res := importSkills(t, a, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusImported {
		t.Fatalf("import after recovery: %+v", res.Items[0])
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("recovered intent must be deleted: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("staging must be discarded by recovery")
	}
}

// TestImportSkillsRecoversCommittedBeforeFinalize proves lazy recovery
// finalizes a committed intent (installs the Baseline, clears the intent)
// before the batch continues; the already-committed entry then reports
// already_imported.
func TestImportSkillsRecoversCommittedBeforeFinalize(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeFinalize(t, a, src, "skills/alpha", "alpha")
	baseline := filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(op.SkillID))
	if _, err := os.Lstat(filepath.Join(baseline, "SKILL.md")); err == nil {
		t.Fatal("test setup must leave the Baseline uninstalled")
	}

	res := importSkills(t, a, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != op.SkillID {
		t.Fatalf("item after committed recovery: %+v", res.Items[0])
	}
	if _, err := os.Lstat(filepath.Join(baseline, "SKILL.md")); err != nil {
		t.Fatalf("recovery must finalize the Baseline: %v", err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("finalized intent must be deleted: %+v", ops)
	}
}

// TestImportSkillsAmbiguousRecoveryBlocksWrites proves an open operation
// whose state cannot be proven stops the batch with CodeRecovery before any
// item processing, preserving every candidate and the intent.
func TestImportSkillsAmbiguousRecoveryBlocksWrites(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeFinalize(t, a, src, "skills/alpha", "alpha")
	// tamper the installed live tree after the commit
	if err := os.WriteFile(filepath.Join(a.StorePath, "alpha", "SKILL.md"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("ambiguous recovery: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].ID != op.ID {
		t.Fatalf("intent must be preserved: %+v", ops)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil || string(data) != "tampered" {
		t.Fatalf("tampered content must be preserved: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); err != nil {
		t.Fatal("operation staging must be preserved")
	}
}
