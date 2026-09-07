package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/source"
)

func TestImportSkillsDefaultSlugConflict(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	srcA := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	importSkills(t, a, srcA.ID, "skills/alpha")

	srcB := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := res.Items[0]
	if item.Status != StatusSkippedConflict || item.Slug != "alpha" {
		t.Fatalf("item: %+v", item)
	}
	if item.Replaces == nil || item.Replaces.Binding == nil || item.Replaces.Binding.SourceID != srcA.ID {
		t.Fatalf("conflict must preview the existing Skill and Binding: %+v", item.Replaces)
	}
	// the Store content is untouched (still A's)
	skills, err := a.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Binding.SourceID != srcA.ID {
		t.Fatalf("skill must remain A's: %+v", skills)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("skipped conflict must leave no intent: %+v", ops)
	}
}

// TestImportSkillsMixedBatchIndependence proves one item's failure never
// rolls back its siblings: a batch returns every outcome independently.
func TestImportSkillsMixedBatchIndependence(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	srcA := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	importSkills(t, a, srcA.ID, "skills/alpha")

	srcB := addLocalSource(t, a, map[string]string{
		"skills/alpha": "Alpha",
		"skills/beta":  "Beta",
		"skills/zed":   "Zed",
	})
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{
			{RelativeDir: "skills/alpha"},
			{RelativeDir: "skills/beta"},
			{RelativeDir: "skills/zed"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("every item must receive an outcome: %+v", res.Items)
	}
	if res.Items[0].Status != StatusSkippedConflict || res.Items[1].Status != StatusImported || res.Items[2].Status != StatusImported {
		t.Fatalf("outcomes: %+v", res.Items)
	}
	if res.Summary.SkippedConflict != 1 || res.Summary.Imported != 2 {
		t.Fatalf("summary: %+v", res.Summary)
	}
	skills, err := a.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 3 {
		t.Fatalf("successful siblings must survive: %+v", skills)
	}
}

// TestImportSkillsGuardFailureIndependent proves a per-Skill guard failure
// fails only its own item while the batch continues.
func TestImportSkillsGuardFailureIndependent(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSkillFile(t, root, "skills/alpha", "Alpha")
	writeSkillFile(t, root, "skills/big", "Big")
	big := filepath.Join(root, "skills", "big", "SKILL.md")
	if err := os.WriteFile(big, []byte("---\nname: Big\ndescription: desc\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(big, os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(51 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root})
	if err != nil {
		t.Fatal(err)
	}

	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{
			{RelativeDir: "skills/alpha"},
			{RelativeDir: "skills/big"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Status != StatusImported {
		t.Fatalf("alpha must import: %+v", res.Items[0])
	}
	if res.Items[1].Status != StatusFailed || res.Items[1].ErrorCode != CodeImportFailed {
		t.Fatalf("big must fail independently: %+v", res.Items[1])
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); err != nil {
		t.Fatalf("alpha live tree: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "big")); !os.IsNotExist(err) {
		t.Fatal("failed item must not leave a live tree")
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("failed item must not leave an intent: %+v", ops)
	}
}

// TestImportSkillsUnmanagedCollisionPreserved proves an unmanaged Store
// directory is never overwritten and its content stays byte-for-byte.
func TestImportSkillsUnmanagedCollisionPreserved(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	live := filepath.Join(a.StorePath, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "precious.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := importSkills(t, a, src.ID, "skills/alpha")
	item := res.Items[0]
	if item.Status != StatusFailed || item.ErrorCode != CodeConflict {
		t.Fatalf("unmanaged collision: %+v", item)
	}
	if data, err := os.ReadFile(filepath.Join(live, "precious.txt")); err != nil || string(data) != "mine" {
		t.Fatalf("unmanaged content must be preserved: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(live, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("the imported tree must not appear inside the unmanaged dir")
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no intent may survive a refused install: %+v", ops)
	}
	if entries, err := os.ReadDir(a.store.InternalDir() + "/staging"); err == nil && len(entries) != 0 {
		t.Fatalf("staging must be discarded: %+v", entries)
	}
}

// TestImportSkillsByteIdenticalUnmanagedCollisionPreserved proves even a
// byte-identical unmanaged directory blocks the import and is preserved.
func TestImportSkillsByteIdenticalUnmanagedCollisionPreserved(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	live := filepath.Join(a.StorePath, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "SKILL.md"),
		[]byte("---\nname: Alpha\ndescription: desc\n---\n# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := importSkills(t, a, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusFailed || res.Items[0].ErrorCode != CodeConflict {
		t.Fatalf("byte-identical unmanaged collision: %+v", res.Items[0])
	}
	if data, err := os.ReadFile(filepath.Join(live, "SKILL.md")); err != nil || len(data) == 0 {
		t.Fatalf("unmanaged content must be preserved: %q, %v", data, err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no intent may survive: %+v", ops)
	}
}

// TestImportSkillsAmbiguousNameRejected proves a non-unique name cannot
// select an entry and fails before any item processing.
func TestImportSkillsAmbiguousNameRejected(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/a1": "Same", "skills/a2": "Same"})
	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{Name: "Same"}},
	}); !isCode(err, CodeInvalidArgument) {
		t.Fatalf("ambiguous name: %v", err)
	}
	if skills, err := a.ListSkills(); err != nil || len(skills) != 0 {
		t.Fatalf("no item may process after a selector failure: %+v, %v", skills, err)
	}
}
