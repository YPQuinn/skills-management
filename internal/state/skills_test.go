package state

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"skillctl/internal/skillstore"
)

func testSkill(slug string) Skill {
	now := time.Now().UTC()
	return Skill{
		Slug:           slug,
		Name:           "Alpha",
		Description:    "one",
		StoreDigest:    "store-digest",
		BaselineDigest: "store-digest",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func testBinding(sourceID, skillID int64) Binding {
	return Binding{
		SkillID:      skillID,
		SourceID:     sourceID,
		RelativeDir:  "skills/alpha",
		Digest:       "store-digest",
		SourceCommit: "abc123",
		ImportedAt:   time.Now().UTC(),
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// testSourceID registers a minimal Source row (bindings reference it) and
// returns its id.
func testSourceID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.Exec(`INSERT INTO sources
		(kind, location, ref, subpath, name, created_at, updated_at, available)
		VALUES ('local', '/tmp/src', '', '', 'src', ?, ?, 1)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// insertOperation inserts a pending operation row and returns it.
func insertOperation(t *testing.T, db *sql.DB, op skillstore.Operation) skillstore.Operation {
	t.Helper()
	inserted, err := InsertOperation(db, op)
	if err != nil {
		t.Fatal(err)
	}
	return inserted
}

func TestSkillInsertGetListRoundTrip(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("insert must return a Skill id")
	}

	skill, binding, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if skill.ID != id || skill.Name != "Alpha" || skill.StoreDigest != "store-digest" {
		t.Fatalf("skill round trip: %+v", skill)
	}
	if binding == nil || binding.SourceID != srcID || binding.RelativeDir != "skills/alpha" || binding.SourceCommit != "abc123" {
		t.Fatalf("binding round trip: %+v", binding)
	}

	skills, err := ListSkills(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Slug != "alpha" {
		t.Fatalf("list: %+v", skills)
	}
}

func TestSkillSlugIsUnique(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	if _, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0)); err != nil {
		t.Fatal(err)
	}
	_, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if !IsUniqueViolation(err) {
		t.Fatalf("duplicate slug: got %v, want a unique violation", err)
	}
}

func TestBindingTupleIsUnique(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	if _, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertSkillAndBinding(db, testSkill("beta"), testBinding(srcID, 0)); !IsUniqueViolation(err) {
		t.Fatal("duplicate (source_id, relative_dir): want a unique violation")
	}
}

func TestUpdateSkillAndBinding(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	updated := testSkill("alpha")
	updated.ID = id
	updated.Name = "Renamed"
	updated.StoreDigest = "new-store-digest"
	b := testBinding(srcID, id)
	b.RelativeDir = "skills/renamed"
	if err := UpdateSkillAndBinding(db, updated, b); err != nil {
		t.Fatal(err)
	}

	skill, binding, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "Renamed" || skill.StoreDigest != "new-store-digest" {
		t.Fatalf("updated skill: %+v", skill)
	}
	if binding.SourceID != srcID || binding.RelativeDir != "skills/renamed" {
		t.Fatalf("updated binding: %+v", binding)
	}
}

// TestUpdateSkillAndBindingRejectsSlugChange proves the update is a
// compare-and-set that cannot silently rename the Skill.
func TestUpdateSkillAndBindingRejectsSlugChange(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	renamed := testSkill("beta")
	renamed.ID = id
	if err := UpdateSkillAndBinding(db, renamed, testBinding(srcID, id)); err == nil {
		t.Fatal("updating with a different slug: want error")
	}
	if _, _, err := GetSkillBySlug(db, "alpha"); err != nil {
		t.Fatalf("the original slug must survive: %v", err)
	}
}

// TestUpdateSkillAndBindingRejectsMissingSkill proves a stale update cannot
// silently apply to zero rows.
func TestUpdateSkillAndBindingRejectsMissingSkill(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	updated := testSkill("alpha")
	updated.ID = 999
	if err := UpdateSkillAndBinding(db, updated, testBinding(srcID, 999)); err == nil {
		t.Fatal("updating a missing Skill: want error")
	}
}

func TestGetSkillBySlugMissing(t *testing.T) {
	db := openTestDB(t)
	_, _, err := GetSkillBySlug(db, "nope")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing skill: got %v, want sql.ErrNoRows", err)
	}
}

// TestSkillPersistenceAcrossReopen proves the Skill, Binding, and journal
// state survive a fresh process opening the database.
func TestSkillPersistenceAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "store-digest",
	})
	if _, err := CommitImport(db, testSkill("alpha"), testBinding(srcID, 0), op.ID); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	skill, binding, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if skill == nil || binding == nil || skill.StoreDigest != "store-digest" {
		t.Fatalf("durable skill: %+v, binding: %+v", skill, binding)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("durable operation: %+v", ops)
	}
}

// TestSnapshotTableExistsInSchema proves migration 005 ships the
// previous-snapshot metadata table the replace path writes.
func TestSnapshotTableExistsInSchema(t *testing.T) {
	db := openTestDB(t)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'skill_snapshots'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("skill_snapshots table missing from migration 005")
	}
}
