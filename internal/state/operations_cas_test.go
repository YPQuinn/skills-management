package state

import (
	"testing"

	"skillctl/internal/skillstore"
)

func TestCommitImportRejectsMismatchedOperation(t *testing.T) {
	tests := []struct {
		name string
		op   skillstore.Operation
	}{
		{
			name: "slug",
			op: skillstore.Operation{
				Slug: "beta", Kind: skillstore.KindImport, NewDigest: "store-digest",
			},
		},
		{
			name: "new digest",
			op: skillstore.Operation{
				Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "other-digest",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			srcID := testSourceID(t, db)
			op := insertOperation(t, db, tt.op)

			if _, err := CommitImport(db, testSkill("alpha"), testBinding(srcID, 0), op.ID); err == nil {
				t.Fatal("committing against a mismatched operation: want error")
			}
			if skills, err := ListSkills(db); err != nil || len(skills) != 0 {
				t.Fatalf("a refused commit must roll back the Skill insert: %+v, %v", skills, err)
			}
			ops, err := ListOpenOperations(db)
			if err != nil {
				t.Fatal(err)
			}
			if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending {
				t.Fatalf("a mismatched operation must remain pending: %+v", ops)
			}
		})
	}
}

func TestCommitReplaceRejectsStaleOperation(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	op := insertOperation(t, db, skillstore.Operation{
		SkillID: id, Slug: "alpha", Kind: skillstore.KindReplace,
		OldDigest: "stale-digest", NewDigest: "new-digest",
	})
	updated := testSkill("alpha")
	updated.ID = id
	updated.StoreDigest = "new-digest"
	updated.BaselineDigest = "new-digest"
	b := testBinding(srcID, id)
	b.Digest = "new-digest"

	if err := CommitReplace(db, updated, b, op.ID); err == nil {
		t.Fatal("committing a stale replace operation: want error")
	}
	skill, binding, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if skill.StoreDigest != "store-digest" || binding.SourceCommit != "abc123" {
		t.Fatalf("a refused replace must roll back Skill and Binding changes: %+v, %+v", skill, binding)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("a stale operation must remain pending: %+v", ops)
	}
}
