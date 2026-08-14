package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
)

// TestImportSkillsFreshAppBlocksPartialReplaceQuarantineDrain proves the
// mid-drain crash state for a pending replace at the application boundary:
// a restore whose quarantine drain was interrupted (content removed, a
// foreign child injected after the crash) cannot be certified, so recovery
// fails with CodeRecovery, the row stays pending, and every candidate
// including the injected child and the recovery slot is preserved.
func TestImportSkillsFreshAppBlocksPartialReplaceQuarantineDrain(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", first.Items[0])
	}
	srcB := addReplacementSource(t, a)
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
	// reproduce the interrupted quarantine drain: the installed live tree
	// sits in the quarantine with its content removed and a foreign child
	// inserted after the crash
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

	fresh := reopenAppUnrecovered(t, a)
	_, err = fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("partial replace quarantine drain: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("the intent must stay pending: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(q, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected foreign child must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "recovery", itoa(op.ID))); err != nil {
		t.Fatal("the recovery slot must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
}

// TestImportSkillsFreshAppBlocksPartialImportCandidateDrain proves the
// mid-candidate-drain crash state for a pending import at the application
// boundary: the candidate no longer matches the proof digest, so recovery
// fails with CodeRecovery, the row stays pending, and the injected foreign
// child and the proof are preserved.
func TestImportSkillsFreshAppBlocksPartialImportCandidateDrain(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeCommit(t, a, src, "skills/alpha", "alpha")
	cand := filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "baseline")
	if err := os.Remove(filepath.Join(cand, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cand, "injected.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := reopenAppUnrecovered(t, a)
	_, err := fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("partial import candidate drain: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("the intent must stay pending: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(cand, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected foreign child must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
}

// TestImportSkillsFreshAppBlocksPartialReplaceCandidateDrain proves the
// same fail-closed refusal for a pending replace: the injected foreign
// child, the proof, and the recovery slot are all preserved.
func TestImportSkillsFreshAppBlocksPartialReplaceCandidateDrain(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", first.Items[0])
	}
	srcB := addReplacementSource(t, a)
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
	cand := filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "baseline")
	if err := os.Remove(filepath.Join(cand, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cand, "injected.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := reopenAppUnrecovered(t, a)
	_, err = fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("partial replace candidate drain: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("the intent must stay pending: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(cand, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected foreign child must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "recovery", itoa(op.ID))); err != nil {
		t.Fatal("the recovery slot must be preserved")
	}
}
