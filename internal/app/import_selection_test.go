package app

import (
	"strings"
	"testing"

	"skillctl/internal/source"
)

func selPath(rel string) ImportSelector { return ImportSelector{RelativeDir: rel} }

func selName(name string) ImportSelector { return ImportSelector{Name: name} }

func TestValidateImportInputAcceptsValid(t *testing.T) {
	t.Parallel()
	good := "explicit-slug"
	for _, in := range []ImportSkillsInput{
		{SourceID: 1, Selectors: []ImportSelector{selPath("skills/a")}},
		{SourceID: 1, Selectors: []ImportSelector{selName("A"), selPath("skills/b")}},
		{SourceID: 1, All: true},
		{SourceID: 1, All: true, AllowLarge: true},
		{SourceID: 1, Selectors: []ImportSelector{{RelativeDir: "skills/a", Slug: &good, Replace: true}}},
	} {
		if err := validateImportInput(in); err != nil {
			t.Fatalf("valid input %+v: %v", in, err)
		}
	}
}

func TestValidateImportInputRejectsCombinations(t *testing.T) {
	t.Parallel()
	slug := "Bad Slug"
	empty := ""
	cases := []struct {
		name string
		in   ImportSkillsInput
	}{
		{"missing source", ImportSkillsInput{Selectors: []ImportSelector{selPath("skills/a")}}},
		{"all with selectors", ImportSkillsInput{SourceID: 1, All: true, Selectors: []ImportSelector{selPath("skills/a")}}},
		{"no selection", ImportSkillsInput{SourceID: 1}},
		{"path and name", ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{{RelativeDir: "skills/a", Name: "A"}}}},
		{"neither path nor name", ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{{}}}},
		{"duplicate path", ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{selPath("skills/a"), selPath("skills/a")}}},
		{"duplicate name", ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{selName("A"), selName("A")}}},
		{"invalid slug", ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{{RelativeDir: "skills/a", Slug: &slug}}}},
		{"empty slug", ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{{RelativeDir: "skills/a", Slug: &empty}}}},
	}
	for _, tc := range cases {
		if err := validateImportInput(tc.in); err == nil {
			t.Fatalf("%s must be rejected", tc.name)
		}
	}
}

func TestResolveImportSelectorsAllSorted(t *testing.T) {
	t.Parallel()
	entries := []source.Entry{
		{RelativeDir: "tools/alpha", Name: "Tools Alpha"},
		{RelativeDir: "skills/alpha", Name: "Alpha"},
		{RelativeDir: "skills/beta", Name: "Beta"},
	}
	out, err := resolveImportSelectors(ImportSkillsInput{SourceID: 1, All: true}, entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].RelativeDir != "skills/alpha" || out[1].RelativeDir != "skills/beta" || out[2].RelativeDir != "tools/alpha" {
		t.Fatalf("all must be sorted by relative directory: %+v", out)
	}
}

func TestResolveImportSelectorsPreservesRequestOrder(t *testing.T) {
	t.Parallel()
	entries := []source.Entry{
		{RelativeDir: "skills/alpha", Name: "Alpha"},
		{RelativeDir: "skills/beta", Name: "Beta"},
		{RelativeDir: "tools/alpha", Name: "Tools Alpha"},
	}
	out, err := resolveImportSelectors(ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{
		selPath("tools/alpha"), selName("Beta"), selPath("skills/alpha"),
	}}, entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].RelativeDir != "tools/alpha" || out[1].RelativeDir != "skills/beta" || out[2].RelativeDir != "skills/alpha" {
		t.Fatalf("explicit selectors must keep request order: %+v", out)
	}
}

func TestResolveImportSelectorsRejectsAmbiguousAndMissing(t *testing.T) {
	t.Parallel()
	entries := []source.Entry{
		{RelativeDir: "skills/alpha", Name: "Alpha"},
		{RelativeDir: "tools/alpha", Name: "Alpha"},
	}
	// an ambiguous name must be resolved by path
	if _, err := resolveImportSelectors(ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{selName("Alpha")}}, entries); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous name: %v", err)
	}
	// a path selects exactly one entry even when the name repeats
	out, err := resolveImportSelectors(ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{selPath("tools/alpha")}}, entries)
	if err != nil || len(out) != 1 || out[0].RelativeDir != "tools/alpha" {
		t.Fatalf("path under a repeated name: %+v, %v", out, err)
	}
	// missing path and missing name
	if _, err := resolveImportSelectors(ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{selPath("skills/nope")}}, entries); err == nil {
		t.Fatal("missing path must fail")
	}
	if _, err := resolveImportSelectors(ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{selName("Nope")}}, entries); err == nil {
		t.Fatal("missing name must fail")
	}
}

func TestResolveImportSelectorsRejectsCrossDuplicate(t *testing.T) {
	t.Parallel()
	entries := []source.Entry{
		{RelativeDir: "skills/beta", Name: "Beta"},
	}
	// a path and a name resolving to the same entry are a duplicate
	_, err := resolveImportSelectors(ImportSkillsInput{SourceID: 1, Selectors: []ImportSelector{
		selPath("skills/beta"), selName("Beta"),
	}}, entries)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("cross path/name duplicate: %v", err)
	}
}
