package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"skillctl/internal/state"
)

// TestImportSkillsReplaceRetainsIdentity proves explicit Replace swaps
// content and Source Binding while retaining the Skill's id and slug, and
// rotates the replaced content into the previous snapshot (tree plus
// metadata).
func TestImportSkillsReplaceRetainsIdentity(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	srcA := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, srcA.ID, "skills/alpha")
	skillID := first.Items[0].SkillID

	// a second Source claims the same slug with different content
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
	before, err := a.ShowSkill(skillID)
	if err != nil {
		t.Fatal(err)
	}

	replace := true
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha", Replace: replace}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := res.Items[0]
	if item.Status != StatusReplaced || item.SkillID != skillID || item.Slug != "alpha" {
		t.Fatalf("replace item: %+v", item)
	}
	if item.Replaces == nil || item.Replaces.ID != skillID || item.Replaces.Binding == nil ||
		item.Replaces.Binding.SourceID != srcA.ID {
		t.Fatalf("replace must preview the superseded Skill and Binding: %+v", item.Replaces)
	}
	after, err := a.ShowSkill(skillID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Slug != "alpha" || after.Binding == nil || after.Binding.SourceID != srcB.ID ||
		after.Binding.RelativeDir != "skills/alpha" {
		t.Fatalf("replaced skill: %+v", after)
	}
	if after.StoreDigest == before.StoreDigest || after.StoreDigest != after.BaselineDigest {
		t.Fatalf("digests after replace: %+v", after)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil || string(data) != "---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n" {
		t.Fatalf("live content after replace: %q, %v", data, err)
	}
	// the previous snapshot tree holds the replaced content
	prev := filepath.Join(a.StorePath, ".skillctl", "previous", strconv.FormatInt(skillID, 10))
	if data, err := os.ReadFile(filepath.Join(prev, "SKILL.md")); err != nil || string(data) == "---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n" {
		t.Fatalf("previous snapshot tree: %q, %v", data, err)
	}
	snap, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Reason != state.SnapshotReasonReplace || snap.Digest != before.StoreDigest {
		t.Fatalf("snapshot metadata: %+v", snap)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("replace must clear its intent: %+v", ops)
	}
}

// TestImportSkillsReplaceFreshSlugImportsNormally proves Replace permission
// on an unclaimed slug is a plain import, not an error.
func TestImportSkillsReplaceFreshSlugImportsNormally(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	replace := true
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha", Replace: replace}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Status != StatusImported {
		t.Fatalf("replace on a fresh slug must import: %+v", res.Items[0])
	}
}

// TestImportSkillsReplaceRejectsChangedStore proves a replace never
// overwrites a live tree whose digest changed after the plan was prepared.
// The refusal is provably pre-mutation, so the staging is discarded with
// the opaque proof and the intent is durably aborted; the edited live
// content is preserved.
func TestImportSkillsReplaceRejectsChangedStore(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	srcA := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, srcA.ID, "skills/alpha")
	skillID := first.Items[0].SkillID
	// external edit after the import
	if err := os.WriteFile(filepath.Join(a.StorePath, "alpha", "SKILL.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootB := t.TempDir()
	writeSkillFile(t, rootB, "skills/alpha", "Alpha")
	srcB, err := a.AddSource(context.Background(), sourceAddInput(rootB))
	if err != nil {
		t.Fatal(err)
	}
	replace := true
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha", Replace: replace}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Status != StatusFailed || res.Items[0].ErrorCode != CodeImportFailed {
		t.Fatalf("changed live must fail as a provable pre-mutation refusal: %+v", res.Items[0])
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil || string(data) != "edited" {
		t.Fatalf("edited content must be preserved: %q, %v", data, err)
	}
	if _, err := a.ShowSkill(skillID); err != nil {
		t.Fatalf("skill row must survive: %v", err)
	}
	// The refusal was provably pre-mutation, so the staging was discarded
	// with the opaque proof and the intent was durably aborted and cleared.
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("the provably aborted intent must be cleared: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging")); err != nil {
		t.Fatal("the layout must survive the discard")
	}
}
