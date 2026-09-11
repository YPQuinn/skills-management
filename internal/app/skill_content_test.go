package app

import (
	"testing"

	"skillctl/internal/source"
)

func TestSkillContent(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	srcID := insertTestSource(t, a, "vercel")
	id := insertTestSkill(t, a, srcID, "alpha", "skills/alpha")
	rewriteStoreFile(t, a, "alpha", "SKILL.md",
		"---\nname: alpha\ndescription: First test skill\nlicense: MIT\n---\n\n# Alpha\n\nBody markdown.\n")

	got, err := a.SkillContent(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.SkillID != id || got.Slug != "alpha" || got.Path != "SKILL.md" {
		t.Fatalf("content: %+v", got)
	}
	wantFields := []source.SkillField{
		{Key: "name", Value: "alpha"},
		{Key: "description", Value: "First test skill"},
		{Key: "license", Value: "MIT"},
	}
	if len(got.Frontmatter) != len(wantFields) {
		t.Fatalf("frontmatter: %+v", got.Frontmatter)
	}
	for i := range wantFields {
		if got.Frontmatter[i] != wantFields[i] {
			t.Fatalf("frontmatter[%d]: got %+v, want %+v", i, got.Frontmatter[i], wantFields[i])
		}
	}
	if got.Body != "# Alpha\n\nBody markdown.\n" {
		t.Fatalf("body: %q", got.Body)
	}

	if _, err := a.SkillContent(9999); !isCode(err, CodeNotFound) {
		t.Fatalf("unknown id: %v", err)
	}

	missing := insertTestSkill(t, a, srcID, "beta", "skills/beta")
	if _, err := a.SkillContent(missing); !isCode(err, CodeNotFound) {
		t.Fatalf("missing Store file: %v", err)
	}
}
