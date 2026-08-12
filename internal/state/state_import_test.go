package state

import (
	"testing"

	"skillctl/internal/skillstore"
)

func TestCommitImportPersistsSkillAndFlipsOperation(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "store-digest",
	})
	if op.ID == 0 || op.Phase != skillstore.PhasePending {
		t.Fatalf("inserted operation: %+v", op)
	}

	id, err := CommitImport(db, testSkill("alpha"), testBinding(srcID, 0), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("commit must return the Skill id")
	}

	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseCommitted || ops[0].SkillID != id {
		t.Fatalf("operation after commit: %+v", ops)
	}
	skill, binding, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if skill.StoreDigest != "store-digest" || binding == nil {
		t.Fatalf("committed skill: %+v, binding: %+v", skill, binding)
	}
}

func TestCommitImportRejectsMissingOperation(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	if _, err := CommitImport(db, testSkill("alpha"), testBinding(srcID, 0), 999); err == nil {
		t.Fatal("committing against a missing operation: want error")
	}
	if _, _, err := GetSkillBySlug(db, "alpha"); err == nil {
		t.Fatal("no Skill may exist after a refused commit")
	}
}

func TestCommitImportRejectsReplay(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "store-digest",
	})
	if _, err := CommitImport(db, testSkill("alpha"), testBinding(srcID, 0), op.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitImport(db, testSkill("beta"), testBinding(srcID, 0), op.ID); err == nil {
		t.Fatal("replaying a committed operation: want error")
	}
}

func TestCommitImportRejectsWrongKind(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindReplace,
		OldDigest: "old", NewDigest: "store-digest",
	})
	if _, err := CommitImport(db, testSkill("alpha"), testBinding(srcID, 0), op.ID); err == nil {
		t.Fatal("import against a replace intent: want error")
	}
}

func TestCommitImportRejectsMismatchedBaseline(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "store-digest",
	})
	s := testSkill("alpha")
	s.BaselineDigest = "different-baseline"
	if _, err := CommitImport(db, s, testBinding(srcID, 0), op.ID); err == nil {
		t.Fatal("commit with mismatched baseline digest: want error")
	}
	if skills, err := ListSkills(db); err != nil || len(skills) != 0 {
		t.Fatalf("a refused commit must roll back: %+v, %v", skills, err)
	}
}

func TestCommitImportRejectsMismatchedBindingDigest(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "store-digest",
	})
	b := testBinding(srcID, 0)
	b.Digest = "different-binding-digest"
	if _, err := CommitImport(db, testSkill("alpha"), b, op.ID); err == nil {
		t.Fatal("commit with mismatched binding digest: want error")
	}
	if skills, err := ListSkills(db); err != nil || len(skills) != 0 {
		t.Fatalf("a refused commit must roll back: %+v, %v", skills, err)
	}
}

func TestCommitImportRejectsEmptyStoreDigest(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport,
	})
	s := testSkill("alpha")
	s.StoreDigest = ""
	s.BaselineDigest = ""
	b := testBinding(srcID, 0)
	b.Digest = ""
	if _, err := CommitImport(db, s, b, op.ID); err == nil {
		t.Fatal("commit with empty store digest: want error")
	}
}
