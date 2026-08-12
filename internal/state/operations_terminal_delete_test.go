package state

import (
	"testing"

	"skillctl/internal/skillstore"
)

func TestDeleteOperationTerminalRequiresExactRow(t *testing.T) {
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

	// a wrong receipt, phase, or identity must not delete the row
	other := op
	other.Receipt = terminalReceipt(9)
	if err := DeleteOperationTerminal(db, other); err == nil {
		t.Fatal("deleting with a different receipt: want error")
	}
	wrongPhase := op
	wrongPhase.Phase = skillstore.PhaseRestored
	if err := DeleteOperationTerminal(db, wrongPhase); err == nil {
		t.Fatal("deleting with a different phase: want error")
	}
	wrongSlug := op
	wrongSlug.Slug = "beta"
	if err := DeleteOperationTerminal(db, wrongSlug); err == nil {
		t.Fatal("deleting with a different slug: want error")
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseFinalized {
		t.Fatalf("refused terminal deletions must keep the row: %+v", ops)
	}

	if err := DeleteOperationTerminal(db, op); err != nil {
		t.Fatal(err)
	}
	ops, err = ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Fatalf("operations after the exact terminal deletion: %+v", ops)
	}
}

func TestDeleteOperationTerminalKeepsNonTerminalRows(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	pending := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
	})
	committed := insertOperation(t, db, skillstore.Operation{
		Slug: "beta", Kind: skillstore.KindImport, NewDigest: "store-digest",
	})
	id, err := CommitImport(db, testSkill("beta"), testBinding(srcID, 1), committed.ID)
	if err != nil {
		t.Fatal(err)
	}
	committed.SkillID = id
	committed.Phase = skillstore.PhaseFinalized
	committed.Receipt = terminalReceipt(1)
	if err := MarkOperationFinalized(db, committed); err != nil {
		t.Fatal(err)
	}
	committed.Phase = skillstore.PhasePending
	if err := DeleteOperationTerminal(db, committed); err == nil {
		t.Fatal("deleting a non-terminal operation: want error")
	}
	committed.Phase = skillstore.PhaseFinalized
	if err := DeleteOperationTerminal(db, committed); err != nil {
		t.Fatal(err)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].ID != pending.ID || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("the pending operation must survive: %+v", ops)
	}
}

// TestMarkOperationFinalizedReplaceMatchesSkillID proves the terminal CAS
// matches the replace operation's pre-commit Skill id exactly.
