package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/source"
)

// importSkills is a shorthand for a single-path import request.
func importSkills(t *testing.T, a *App, srcID int64, relDir string) *ImportSkillsResult {
	t.Helper()
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcID, Selectors: []ImportSelector{{RelativeDir: relDir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestImportSkillsPathSelector(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	digest := findEntry(t, src, "skills/alpha").Digest

	res := importSkills(t, a, src.ID, "skills/alpha")
	if len(res.Items) != 1 || res.Items[0].Status != StatusImported {
		t.Fatalf("items: %+v", res.Items)
	}
	item := res.Items[0]
	if item.RelativeDir != "skills/alpha" || item.Slug != "alpha" || item.SkillID == 0 {
		t.Fatalf("item: %+v", item)
	}
	live := filepath.Join(a.StorePath, "alpha")
	if data, err := os.ReadFile(filepath.Join(live, "SKILL.md")); err != nil || len(data) == 0 {
		t.Fatalf("live tree: %q, %v", data, err)
	}
	baseline := filepath.Join(a.StorePath, ".skillctl", "baselines", fmt.Sprint(item.SkillID))
	if _, err := os.Lstat(filepath.Join(baseline, "SKILL.md")); err != nil {
		t.Fatalf("baseline missing: %v", err)
	}
	skills, err := a.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Binding == nil || skills[0].Binding.SourceID != src.ID ||
		skills[0].Binding.RelativeDir != "skills/alpha" || skills[0].StoreDigest != digest {
		t.Fatalf("skill after import: %+v", skills[0])
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("intents must be cleared: %+v", ops)
	}
}

func TestImportSkillsNameSelector(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{Name: "Alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Status != StatusImported || res.Items[0].Slug != "alpha" {
		t.Fatalf("items: %+v", res.Items)
	}
}

func TestImportSkillsAllSorted(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/beta": "Beta", "skills/alpha": "Alpha"})
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 || res.Items[0].Slug != "alpha" || res.Items[1].Slug != "beta" {
		t.Fatalf("All must import every entry sorted by path: %+v", res.Items)
	}
	for _, it := range res.Items {
		if it.Status != StatusImported {
			t.Fatalf("item: %+v", it)
		}
	}
}

// TestImportSkillsOneFreshObservationPerBatch proves a multi-entry batch
// performs exactly one Source observation, which is also persisted.
func TestImportSkillsOneFreshObservationPerBatch(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/a": "A", "skills/b": "B"})
	before, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	a.observer = countingObserver{Observer: a.observer, n: &n}

	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("batch observations: %d, want exactly 1", n)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items: %+v", res.Items)
	}
	after, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Available || after.LastCheckResult != source.CheckResultOK ||
		after.LastInventoryDigest == "" || len(after.Entries) != 2 ||
		after.LastCheckedAt == nil || !after.LastCheckedAt.After(*before.LastCheckedAt) {
		t.Fatalf("fresh observation must be persisted: %+v", after)
	}
}

func TestImportSkillsSlugOverride(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	override := "custom"
	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha", Slug: &override}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Slug != "custom" || res.Items[0].RequestedSlug != "custom" {
		t.Fatalf("item: %+v", res.Items[0])
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "custom")); err != nil {
		t.Fatalf("live dir must use the override: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); !os.IsNotExist(err) {
		t.Fatal("no default-slug dir may exist")
	}
}

func TestImportSkillsDefaultSlugNormalizes(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/Code Review": "Code Review"})
	res := importSkills(t, a, src.ID, "skills/Code Review")
	if res.Items[0].Slug != "code-review" {
		t.Fatalf("slug: %q", res.Items[0].Slug)
	}
}

func TestImportSkillsRepeatIsAlreadyImported(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	second := importSkills(t, a, src.ID, "skills/alpha")
	if second.Items[0].Status != StatusAlreadyImported || second.Items[0].SkillID != first.Items[0].SkillID {
		t.Fatalf("repeat: %+v", second.Items[0])
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no-op must not leave intents: %+v", ops)
	}
}

func TestImportSkillsRequestValidation(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	cases := []ImportSkillsInput{
		{SourceID: src.ID, All: true, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}}},
		{SourceID: src.ID},
		{SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}, {RelativeDir: "skills/alpha"}}},
		{SourceID: 0, All: true},
		{SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/missing"}}},
	}
	for i, in := range cases {
		if _, err := a.ImportSkills(context.Background(), in); !isCode(err, CodeInvalidArgument) {
			t.Fatalf("case %d: want invalid_argument, got %v", i, err)
		}
	}
	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{SourceID: 9999, All: true}); !isCode(err, CodeNotFound) {
		t.Fatalf("missing source: %v", err)
	}
}

func TestImportSkillsSkillQueries(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	importSkills(t, a, src.ID, "skills/alpha")

	skills, err := a.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	id := skills[0].ID
	shown, err := a.ShowSkill(id)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Slug != "alpha" || shown.Binding == nil || shown.Binding.SourceName == "" {
		t.Fatalf("show: %+v", shown)
	}
	if rid, err := a.ResolveSkillArg("alpha"); err != nil || rid != id {
		t.Fatalf("resolve by slug: %d, %v", rid, err)
	}
	if rid, err := a.ResolveSkillArg(fmt.Sprint(id)); err != nil || rid != id {
		t.Fatalf("resolve by id: %d, %v", rid, err)
	}
	if _, err := a.ShowSkill(4242); !isCode(err, CodeNotFound) {
		t.Fatalf("show unknown: %v", err)
	}
	if _, err := a.ResolveSkillArg("nope"); !isCode(err, CodeNotFound) {
		t.Fatalf("resolve unknown: %v", err)
	}
}
