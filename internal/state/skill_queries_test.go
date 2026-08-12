package state

import (
	"database/sql"
	"errors"
	"testing"
)

func TestSkillDetailQueriesRoundTrip(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}

	byID, err := GetSkillDetailByID(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if byID.Skill.Slug != "alpha" || byID.Skill.StoreDigest != "store-digest" {
		t.Fatalf("detail by id: %+v", byID)
	}
	if byID.Binding == nil || byID.Binding.SourceID != srcID || byID.Binding.SourceName != "src" ||
		byID.Binding.RelativeDir != "skills/alpha" || byID.Binding.SourceCommit != "abc123" {
		t.Fatalf("binding by id: %+v", byID.Binding)
	}

	bySlug, err := GetSkillDetailBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if bySlug.Skill.ID != id || bySlug.Binding.SourceName != "src" {
		t.Fatalf("detail by slug: %+v", bySlug)
	}

	byEntry, err := GetSkillBySourceEntry(db, srcID, "skills/alpha")
	if err != nil {
		t.Fatal(err)
	}
	if byEntry.Skill.ID != id || byEntry.Binding.SkillID != id {
		t.Fatalf("detail by source entry: %+v", byEntry)
	}
}

func TestSkillDetailUnboundHasNilBinding(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	if _, err := InsertSkillAndBinding(db, testSkill("solo"), testBinding(srcID, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM source_bindings`); err != nil {
		t.Fatal(err)
	}

	d, err := GetSkillDetailBySlug(db, "solo")
	if err != nil {
		t.Fatal(err)
	}
	if d.Binding != nil {
		t.Fatalf("unbound Skill must have a nil Binding: %+v", d.Binding)
	}
}

func TestSkillDetailListOrdering(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	for _, slug := range []string{"zeta", "alpha", "beta"} {
		b := testBinding(srcID, 0)
		b.RelativeDir = "skills/" + slug
		if _, err := InsertSkillAndBinding(db, testSkill(slug), b); err != nil {
			t.Fatal(err)
		}
	}

	items, err := ListSkillDetails(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 ||
		items[0].Skill.Slug != "alpha" || items[1].Skill.Slug != "beta" || items[2].Skill.Slug != "zeta" {
		t.Fatalf("list order: %+v", items)
	}
	for _, d := range items {
		if d.Binding == nil || d.Binding.SourceName != "src" {
			t.Fatalf("list binding: %+v", d.Binding)
		}
	}
}

func TestSkillDetailMissingRows(t *testing.T) {
	db := openTestDB(t)
	if _, err := GetSkillDetailByID(db, 42); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing by id: %v", err)
	}
	if _, err := GetSkillDetailBySlug(db, "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing by slug: %v", err)
	}
	if _, err := GetSkillBySourceEntry(db, 42, "skills/x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing by source entry: %v", err)
	}
}
