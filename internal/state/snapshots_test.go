package state

import (
	"testing"

	"skillctl/internal/skillstore"
)

func TestCommitReplaceFlipsOperationAndRecordsSnapshot(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	op := insertOperation(t, db, skillstore.Operation{
		SkillID: id, Slug: "alpha", Kind: skillstore.KindReplace,
		OldDigest: "store-digest", NewDigest: "new-digest",
	})
	updated := testSkill("alpha")
	updated.ID = id
	updated.StoreDigest = "new-digest"
	updated.BaselineDigest = "new-digest"
	b := testBinding(srcID, id)
	b.Digest = "new-digest"
	b.SourceCommit = "def456"
	if err := CommitReplace(db, updated, b, op.ID); err != nil {
		t.Fatal(err)
	}

	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("operation after replace: %+v", ops)
	}
	skill, binding, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if skill.StoreDigest != "new-digest" || binding.SourceID != srcID {
		t.Fatalf("replaced skill: %+v, binding: %+v", skill, binding)
	}
	// the previous-snapshot metadata was committed atomically with the
	// replace: the replaced digest and the binding's Source evidence
	snap, err := GetSnapshot(db, id)
	if err != nil {
		t.Fatalf("snapshot metadata missing after replace: %v", err)
	}
	if snap.Digest != "store-digest" || snap.SourceCommit != "abc123" || snap.Reason != SnapshotReasonReplace {
		t.Fatalf("snapshot metadata: %+v", snap)
	}
	if snap.CreatedAt.IsZero() {
		t.Fatal("snapshot time missing")
	}
}

// TestCommitReplaceSupersedesSnapshot proves at most one snapshot row
// exists per Skill and a later replace supersedes it.
func TestCommitReplaceSupersedesSnapshot(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	first := insertOperation(t, db, skillstore.Operation{
		SkillID: id, Slug: "alpha", Kind: skillstore.KindReplace,
		OldDigest: "store-digest", NewDigest: "new-digest",
	})
	updated := testSkill("alpha")
	updated.ID = id
	updated.StoreDigest = "new-digest"
	updated.BaselineDigest = "new-digest"
	b1 := testBinding(srcID, id)
	b1.Digest = "new-digest"
	if err := CommitReplace(db, updated, b1, first.ID); err != nil {
		t.Fatal(err)
	}
	second := insertOperation(t, db, skillstore.Operation{
		SkillID: id, Slug: "alpha", Kind: skillstore.KindReplace,
		OldDigest: "new-digest", NewDigest: "newer-digest",
	})
	updated.StoreDigest = "newer-digest"
	updated.BaselineDigest = "newer-digest"
	b2 := testBinding(srcID, id)
	b2.Digest = "newer-digest"
	if err := CommitReplace(db, updated, b2, second.ID); err != nil {
		t.Fatal(err)
	}
	snap, err := GetSnapshot(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != "new-digest" {
		t.Fatalf("snapshot must be superseded: %+v", snap)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM skill_snapshots WHERE skill_id = ?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("at most one snapshot row per Skill, got %d", count)
	}
}

// TestCommitImportRecordsNoSnapshot proves an initial import never creates
// previous-snapshot metadata.
func TestCommitImportRecordsNoSnapshot(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "store-digest",
	})
	id, err := CommitImport(db, testSkill("alpha"), testBinding(srcID, 0), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GetSnapshot(db, id); err == nil {
		t.Fatal("an initial import must not record a previous snapshot")
	}
}
