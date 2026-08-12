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

// replaceAndStopAfterReceiptPersisted builds a committed replace whose
// semantic finalization completed and whose terminal receipt persisted,
// with the receipt-bound evidence retained.
func replaceAndStopAfterReceiptPersisted(t *testing.T, a *App, src *source.Source, relDir, slug string) skillstore.Operation {
	t.Helper()
	op := replaceAndCommit(t, a, src, relDir, slug)
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

// TestImportSkillsFreshAppConvergesReplaceTerminalRow proves the replace
// finalize crash window at the application boundary: the Baseline advance
// and the previous-snapshot rotation completed, the receipt persisted, and
// the fresh App validates the receipt-bound terminal objects (live,
// Baseline, and rotated previous), cleans the evidence, and clears the row.
func TestImportSkillsFreshAppConvergesReplaceTerminalRow(t *testing.T) {
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
	replaceAndStopAfterReceiptPersisted(t, a, srcB, "skills/alpha", "alpha")

	fresh := reopenApp(t, a)
	// lazy recovery on the next batch converges the terminal replace row
	res := importSkills(t, fresh, srcB.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != skillID {
		t.Fatalf("item after replace terminal convergence: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the replace terminal row must be cleared: %+v", ops)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil || string(data) != "---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n" {
		t.Fatalf("the replaced live tree must survive: %q, %v", data, err)
	}
	prev := filepath.Join(a.StorePath, ".skillctl", "previous", itoa(skillID))
	if data, err := os.ReadFile(filepath.Join(prev, "SKILL.md")); err != nil || string(data) == "---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n" {
		t.Fatalf("the rotated previous snapshot must survive: %q, %v", data, err)
	}
}

// TestImportSkillsFreshAppConvergesReplaceRestoreResume proves the replace
// restore crash window at the application boundary: the semantic restore
// already moved the recovery slot back to live before the receipt
// persisted. The fresh App's recovery resumes only by the live tree's
// physical identity matching the proof's recovery identity, persists the
// receipt, cleans the evidence, and clears the row.
func TestImportSkillsFreshAppConvergesReplaceRestoreResume(t *testing.T) {
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
	// crash window: the semantic restore completed, the receipt never
	// persisted, and the row is still pending
	if _, err := a.store.PrepareRestore(context.Background(), op); err != nil {
		t.Fatal(err)
	}

	fresh := reopenApp(t, a)
	again := importSkills(t, fresh, src.ID, "skills/alpha")
	if again.Items[0].Status != StatusAlreadyImported || again.Items[0].SkillID != skillID {
		t.Fatalf("item after resumed replace restore: %+v", again.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the replace restore row must be cleared: %+v", ops)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil ||
		string(data) == "---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n" {
		t.Fatalf("the restored old content must be live: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("the resumed restore must clean the evidence")
	}
}

// copyDirForTest copies the tree at src into dst (fresh).
func copyDirForTest(dst, src string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {

			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
