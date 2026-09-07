package app

import (
	"testing"
	"time"

	"skillctl/internal/state"
)

// insertTestSource registers a minimal Source row and returns its id.
func insertTestSource(t *testing.T, a *App, name string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := a.db.Exec(`INSERT INTO sources
		(kind, location, ref, subpath, name, created_at, updated_at, available)
		VALUES ('local', '/tmp/src', '', '', ?, ?, ?, 1)`, name, now, now)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// insertTestSkill persists one committed Skill with its Binding and returns
// the Skill id.
func insertTestSkill(t *testing.T, a *App, srcID int64, slug, relativeDir string) int64 {
	t.Helper()
	now := time.Now().UTC()
	id, err := state.InsertSkillAndBinding(a.db, state.Skill{
		Slug: slug, Name: "Alpha", Description: "one",
		StoreDigest: "store-digest", BaselineDigest: "store-digest",
		CreatedAt: now, UpdatedAt: now,
	}, state.Binding{
		SourceID: srcID, RelativeDir: relativeDir, Digest: "store-digest",
		SourceCommit: "abc123", ImportedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestListSkillsIncludesBindingAndSourceName(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	srcID := insertTestSource(t, a, "vercel")
	insertTestSkill(t, a, srcID, "alpha", "skills/alpha")
	insertTestSkill(t, a, srcID, "beta", "skills/beta")

	skills, err := a.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 || skills[0].Slug != "alpha" || skills[1].Slug != "beta" {
		t.Fatalf("list order: %+v", skills)
	}
	b := skills[0].Binding
	if b == nil || b.SourceName != "vercel" || b.SourceID != srcID ||
		b.RelativeDir != "skills/alpha" || b.SourceCommit != "abc123" {
		t.Fatalf("binding: %+v", b)
	}
	if skills[0].StoreDigest != "store-digest" || skills[0].BaselineDigest != "store-digest" ||
		skills[0].Name != "Alpha" || skills[0].ID == 0 {
		t.Fatalf("skill: %+v", skills[0])
	}
}

func TestListSkillsUnboundBindingIsNil(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	srcID := insertTestSource(t, a, "vercel")
	id := insertTestSkill(t, a, srcID, "alpha", "skills/alpha")
	if _, err := a.db.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, id); err != nil {
		t.Fatal(err)
	}

	skills, err := a.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Binding != nil || skills[0].Slug != "alpha" {
		t.Fatalf("unbound skill: %+v", skills)
	}
}

func TestShowSkill(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	srcID := insertTestSource(t, a, "vercel")
	id := insertTestSkill(t, a, srcID, "alpha", "skills/alpha")

	shown, err := a.ShowSkill(id)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Slug != "alpha" || shown.Binding == nil || shown.Binding.SourceName != "vercel" {
		t.Fatalf("show: %+v", shown)
	}

	if _, err := a.ShowSkill(9999); !isCode(err, CodeNotFound) {
		t.Fatalf("show missing: %v", err)
	}
}

func TestResolveSkillArg(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	srcID := insertTestSource(t, a, "vercel")
	id := insertTestSkill(t, a, srcID, "alpha", "skills/alpha")

	if got, err := a.ResolveSkillArg("alpha"); err != nil || got != id {
		t.Fatalf("resolve by slug: %d, %v", got, err)
	}
	// a numeric argument is the id itself, even without a matching row
	if got, err := a.ResolveSkillArg("9999"); err != nil || got != 9999 {
		t.Fatalf("numeric id must win over slug lookup: %d, %v", got, err)
	}
	if _, err := a.ResolveSkillArg("missing"); !isCode(err, CodeNotFound) {
		t.Fatalf("resolve missing: %v", err)
	}
}
