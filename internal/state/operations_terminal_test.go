package state

import (
	"testing"

	"skillctl/internal/skillstore"
)

// TestDeleteOperationIntentRemovesExactPendingAndAbortedIntents proves the
// full-identity/phase CAS deletion: the row must still carry the same
// identity and phase, so an ID-only reference can never clear a stale or
// foreign row.
func TestDeleteOperationIntentRemovesExactIntents(t *testing.T) {
	for _, phase := range []string{skillstore.PhasePending, skillstore.PhaseAborted} {
		t.Run(phase, func(t *testing.T) {
			db := openTestDB(t)
			op := insertOperation(t, db, skillstore.Operation{
				Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
			})
			if phase == skillstore.PhaseAborted {
				if err := MarkOperationAborted(db, op); err != nil {
					t.Fatal(err)
				}
				op.Phase = skillstore.PhaseAborted
			}
			if err := DeleteOperationIntent(db, op); err != nil {
				t.Fatal(err)
			}
			if ops, err := ListOpenOperations(db); err != nil || len(ops) != 0 {
				t.Fatalf("intent must be cleared: %+v, %v", ops, err)
			}
		})
	}
}

func TestDeleteOperationIntentRejectsMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*skillstore.Operation)
	}{
		{name: "wrong id", mutate: func(op *skillstore.Operation) { op.ID = 999 }},
		{name: "wrong slug", mutate: func(op *skillstore.Operation) { op.Slug = "beta" }},
		{name: "wrong kind", mutate: func(op *skillstore.Operation) { op.Kind = skillstore.KindReplace }},
		{name: "wrong old digest", mutate: func(op *skillstore.Operation) { op.OldDigest = "stale" }},
		{name: "wrong new digest", mutate: func(op *skillstore.Operation) { op.NewDigest = "other" }},
		{name: "wrong skill id", mutate: func(op *skillstore.Operation) { op.SkillID = 4242 }},
		{name: "wrong phase", mutate: func(op *skillstore.Operation) { op.Phase = skillstore.PhaseAborted }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			op := insertOperation(t, db, skillstore.Operation{
				Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
			})
			mutated := op
			tt.mutate(&mutated)

			if err := DeleteOperationIntent(db, mutated); err == nil {
				t.Fatalf("%s: want error", tt.name)
			}
			ops, err := ListOpenOperations(db)
			if err != nil {
				t.Fatal(err)
			}
			if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhasePending {
				t.Fatalf("a refused delete must keep the intent: %+v", ops)
			}
		})
	}
}

// terminalReceipt returns a non-empty opaque receipt payload for tests.
func terminalReceipt(v byte) []byte {
	return []byte{'r', v}
}

func TestMarkOperationFinalizedBindsReceipt(t *testing.T) {
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
	op.Phase = skillstore.PhaseFinalized
	op.Receipt = terminalReceipt(1)

	if err := MarkOperationFinalized(db, op); err != nil {
		t.Fatal(err)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseFinalized ||
		string(ops[0].Receipt) != string(terminalReceipt(1)) {
		t.Fatalf("finalized operation: %+v", ops)
	}
}

func TestMarkOperationFinalizedRejectsMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*skillstore.Operation)
	}{
		{name: "wrong id", mutate: func(op *skillstore.Operation) { op.ID = 999 }},
		{name: "wrong slug", mutate: func(op *skillstore.Operation) { op.Slug = "beta" }},
		{name: "wrong kind", mutate: func(op *skillstore.Operation) { op.Kind = skillstore.KindReplace }},
		{name: "wrong old digest", mutate: func(op *skillstore.Operation) { op.OldDigest = "stale" }},
		{name: "wrong new digest", mutate: func(op *skillstore.Operation) { op.NewDigest = "other" }},
		{name: "wrong skill id", mutate: func(op *skillstore.Operation) { op.SkillID = 4242 }},
		{name: "wrong source phase", mutate: func(op *skillstore.Operation) { op.Phase = skillstore.PhaseRestored }},
		{name: "missing receipt", mutate: func(op *skillstore.Operation) { op.Receipt = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			op.Phase = skillstore.PhaseFinalized
			op.Receipt = terminalReceipt(1)
			mutated := op
			tt.mutate(&mutated)

			if err := MarkOperationFinalized(db, mutated); err == nil {
				t.Fatalf("%s: want error", tt.name)
			}
			ops, err := ListOpenOperations(db)
			if err != nil {
				t.Fatal(err)
			}
			if len(ops) != 1 || ops[0].Phase != skillstore.PhaseCommitted || ops[0].Receipt != nil {
				t.Fatalf("a refused finalize must leave the operation committed without a receipt: %+v", ops)
			}
		})
	}
}

func TestMarkOperationRestoredBindsReceipt(t *testing.T) {
	db := openTestDB(t)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
	})
	op.Phase = skillstore.PhaseRestored
	op.Receipt = terminalReceipt(2)

	if err := MarkOperationRestored(db, op); err != nil {
		t.Fatal(err)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseRestored ||
		string(ops[0].Receipt) != string(terminalReceipt(2)) {
		t.Fatalf("restored operation: %+v", ops)
	}
}

func TestMarkOperationRestoredRejectsMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*skillstore.Operation)
	}{
		{name: "wrong id", mutate: func(op *skillstore.Operation) { op.ID = 999 }},
		{name: "wrong slug", mutate: func(op *skillstore.Operation) { op.Slug = "beta" }},
		{name: "wrong kind", mutate: func(op *skillstore.Operation) { op.Kind = skillstore.KindReplace }},
		{name: "wrong source phase", mutate: func(op *skillstore.Operation) { op.Phase = skillstore.PhaseFinalized }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			op := insertOperation(t, db, skillstore.Operation{
				Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
			})
			op.Phase = skillstore.PhaseRestored
			op.Receipt = terminalReceipt(2)
			mutated := op
			tt.mutate(&mutated)

			if err := MarkOperationRestored(db, mutated); err == nil {
				t.Fatalf("%s: want error", tt.name)
			}
			ops, err := ListOpenOperations(db)
			if err != nil {
				t.Fatal(err)
			}
			if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending {
				t.Fatalf("a refused restore must leave the operation pending: %+v", ops)
			}
		})
	}
}

// TestMarkOperationFinalizedRejectsPending proves the terminal CAS only
// certifies a committed row: a pending row with the same identity is never
// finalized.
func TestMarkOperationFinalizedRejectsPending(t *testing.T) {
	db := openTestDB(t)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
	})
	op.Phase = skillstore.PhaseFinalized
	op.Receipt = terminalReceipt(1)
	if err := MarkOperationFinalized(db, op); err == nil {
		t.Fatal("finalizing a pending operation: want error")
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending || ops[0].Receipt != nil {
		t.Fatalf("a refused finalize must leave the operation pending: %+v", ops)
	}
}

func TestMarkOperationRestoredRejectsCommitted(t *testing.T) {
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
	op.Phase = skillstore.PhaseRestored
	op.Receipt = terminalReceipt(2)
	if err := MarkOperationRestored(db, op); err == nil {
		t.Fatal("restoring a committed operation: want error")
	}
}

func TestMarkOperationFinalizedReplaceMatchesSkillID(t *testing.T) {
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
	if err := CommitReplace(db, updated, b, op.ID); err != nil {
		t.Fatal(err)
	}
	op.Phase = skillstore.PhaseFinalized
	op.Receipt = terminalReceipt(1)
	if err := MarkOperationFinalized(db, op); err != nil {
		t.Fatal(err)
	}
	op.SkillID = 4242
	if err := MarkOperationFinalized(db, op); err == nil {
		t.Fatal("finalizing a replace with a stale Skill id: want error")
	}
}
