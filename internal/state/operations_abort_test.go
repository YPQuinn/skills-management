package state

import (
	"testing"

	"skillctl/internal/skillstore"
)

func TestMarkOperationAbortedFlipsPendingPhase(t *testing.T) {
	db := openTestDB(t)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
	})
	if err := MarkOperationAborted(db, op); err != nil {
		t.Fatal(err)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseAborted {
		t.Fatalf("operation after abort: %+v", ops)
	}
}

func TestMarkOperationAbortedRejectsMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*skillstore.Operation)
	}{
		{name: "slug", mutate: func(op *skillstore.Operation) { op.Slug = "beta" }},
		{name: "new digest", mutate: func(op *skillstore.Operation) { op.NewDigest = "other" }},
		{name: "kind", mutate: func(op *skillstore.Operation) { op.Kind = skillstore.KindReplace }},
		{name: "wrong id", mutate: func(op *skillstore.Operation) { op.ID = 999 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			op := insertOperation(t, db, skillstore.Operation{
				Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
			})
			mutated := op
			tt.mutate(&mutated)
			if err := MarkOperationAborted(db, mutated); err == nil {
				t.Fatal("aborting a mismatched operation: want error")
			}
			ops, err := ListOpenOperations(db)
			if err != nil {
				t.Fatal(err)
			}
			if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending {
				t.Fatalf("a refused abort must leave the operation pending: %+v", ops)
			}
		})
	}
}

func TestMarkOperationAbortedRejectsCommitted(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "store-digest",
	})
	id, err := CommitImport(db, testSkill("alpha"), testBinding(srcID, 0), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	op.SkillID = id
	if err := MarkOperationAborted(db, op); err == nil {
		t.Fatal("aborting a committed operation: want error")
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("a refused abort must leave the operation committed: %+v", ops)
	}
}

func TestMarkOperationAbortedReplaceMatchesSkillID(t *testing.T) {
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
	if err := MarkOperationAborted(db, op); err != nil {
		t.Fatal(err)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseAborted || ops[0].SkillID != id {
		t.Fatalf("replace abort must match the pre-commit Skill id: %+v", ops)
	}
	// a stale Skill id on the same replace refuses
	op.SkillID = 4242
	if err := MarkOperationAborted(db, op); err == nil {
		t.Fatal("aborting a replace with a stale Skill id: want error")
	}
}
