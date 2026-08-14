package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
)

// TestImportSkillsFreshAppBlocksForeignBaselineSwappedAtEvidenceRemoval
// proves the post-cleanup terminal validation for a replace's installed
// Baseline at the application seam: a byte-identical foreign Baseline
// substituted at the final removal hook is refused with CodeRecovery, the
// terminal row and its receipt are kept, and the foreign object is
// preserved.
func TestImportSkillsFreshAppBlocksForeignBaselineSwappedAtEvidenceRemoval(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", first.Items[0])
	}
	srcB := addReplacementSource(t, a)
	op := replaceAndStopAfterReceiptPersisted(t, a, srcB, "skills/alpha", "alpha")

	fresh := reopenAppUnrecovered(t, a)
	fresh.store.SetHook(func(p skillstore.HookPoint) {
		if p != skillstore.HookAfterRemoveOp {
			return
		}
		base := filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(op.SkillID))
		real := base + ".real"
		if err := os.Rename(base, real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyDirForTest(swap, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, base); err != nil {
			t.Fatal(err)
		}
	})
	defer fresh.store.SetHook(nil)

	_, err := fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("foreign Baseline at the evidence-removal boundary: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseFinalized {
		t.Fatalf("the terminal row must be kept with its receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(op.SkillID), "SKILL.md")); err != nil {
		t.Fatal("the byte-identical foreign Baseline must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(op.SkillID)+".real", "SKILL.md")); err != nil {
		t.Fatal("the installed Baseline must survive beside the foreign one")
	}
}

// TestImportSkillsBlocksForeignLiveAfterRestoreEvidenceRemoval proves a
// restored import's terminal expectation holds at the application seam: a
// foreign live tree that appears at the final evidence-removal hook is
// refused, the restored row with its receipt is kept, and the foreign tree
// is preserved.
func TestImportSkillsBlocksForeignLiveAfterRestoreEvidenceRemoval(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			cancel()
		}
	}
	a.store.SetHook(func(p skillstore.HookPoint) {
		if p != skillstore.HookAfterRemoveOp {
			return
		}
		live := filepath.Join(a.StorePath, "alpha")
		if err := os.MkdirAll(live, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	defer a.store.SetHook(nil)

	res, err := a.ImportSkills(ctx, ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Status != StatusFailed || res.Items[0].ErrorCode != CodeRecovery {
		t.Fatalf("foreign live after the restore removal: %+v", res.Items[0])
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseRestored || len(ops[0].Receipt) == 0 {
		t.Fatalf("the restored row must be kept with its receipt: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the foreign live tree must be preserved: %q, %v", data, rerr)
	}
}

// TestImportSkillsBlocksForeignLiveAfterReplaceRestoreEvidenceRemoval
// proves a restored replace's terminal expectation holds at the
// application seam: a byte-identical foreign live tree substituted at the
// final evidence-removal hook is refused, the restored row with its
// receipt is kept, and both trees are preserved.
func TestImportSkillsBlocksForeignLiveAfterReplaceRestoreEvidenceRemoval(t *testing.T) {
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
	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			cancel()
		}
	}
	a.store.SetHook(func(p skillstore.HookPoint) {
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
	defer a.store.SetHook(nil)

	_, err = a.ImportSkills(ctx, ImportSkillsInput{
		SourceID:  srcB.ID,
		Selectors: []ImportSelector{{RelativeDir: "skills/alpha", Slug: strPtr("alpha"), Replace: true}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("foreign live after the replace restore removal: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseRestored || len(ops[0].Receipt) == 0 {
		t.Fatalf("the restored replace row must be kept with its receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil {
		t.Fatal("the byte-identical foreign live tree must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha.real", "SKILL.md")); err != nil {
		t.Fatal("the restored live tree must survive beside the foreign one")
	}
}
