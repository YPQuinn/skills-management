package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// baselineAndStopBeforeFinalize stages and SQLite-commits one Baseline-only
// refresh (the Keep Store / convergence journal shape) and returns the
// committed operation, leaving the Baseline swap and the evidence cleanup
// for recovery — the post-commit, pre-finalize crash window.
func baselineAndStopBeforeFinalize(t *testing.T, a *App, src *source.Source, relDir, slug string) skillstore.Operation {
	t.Helper()
	dir, digest := materializeTo(t, a, src, relDir)
	detail, err := state.GetSkillDetailBySlug(a.db, slug)
	if err != nil {
		t.Fatal(err)
	}
	op := skillstore.Operation{
		SkillID: detail.Skill.ID, Slug: detail.Skill.Slug, Kind: skillstore.KindBaseline,
		OldDigest: detail.Skill.BaselineDigest, NewDigest: digest,
	}
	op, err = state.InsertOperation(a.db, op)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Stage(context.Background(), op.ID, dir, digest, true); err != nil {
		t.Fatal(err)
	}
	if err := state.CommitBaselineAdvance(a.db, op, detail.Skill.ID, detail.Skill.BaselineDigest, digest, src.LastCommit); err != nil {
		t.Fatal(err)
	}
	op.SkillID = detail.Skill.ID
	return op
}

// TestAppNewRecoversCommittedBaselineBeforeShowDiff locks the open-time
// recovery: a crash after a Baseline advance committed but before its
// finalize leaves a committed journal row with staging evidence. The fresh
// App resolves it while opening — before any show or diff runs — so the
// read-only views already report the recovered Baseline and the journal is
// clean without any Store write.
func TestAppNewRecoversCommittedBaselineBeforeShowDiff(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	src = checkSourceNow(t, a, src)
	op := baselineAndStopBeforeFinalize(t, a, src, "skills/demo", "demo")

	// The committed row and its staging evidence must survive the crash.
	if ops := openOperations(t, a); len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("the committed row must survive: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "tree")); err != nil {
		t.Fatal("the staging evidence must survive the crash")
	}

	fresh := reopenApp(t, a)
	// Open-time recovery already finalized the Baseline and cleared the
	// journal — before any capability ran.
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("open must recover the journal before any capability runs: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("the receipt-bound evidence must be cleaned at open")
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID), "notes.md")); err != nil ||
		string(data) != "upstream\n" {
		t.Fatalf("the Baseline tree must be swapped at open: %q, %v", data, err)
	}

	// The read-only views then report the recovered state.
	shown, err := fresh.ShowSkill(skillID)
	if err != nil {
		t.Fatal(err)
	}
	if shown.BaselineDigest != op.NewDigest {
		t.Fatalf("show must report the recovered Baseline: %q, want %q", shown.BaselineDigest, op.NewDigest)
	}
	diff, err := fresh.DiffSkill(context.Background(), skillID, "")
	if err != nil {
		t.Fatal(err)
	}
	if diff.BaselineDigest != op.NewDigest {
		t.Fatalf("diff must report the recovered Baseline: %q, want %q", diff.BaselineDigest, op.NewDigest)
	}
}
