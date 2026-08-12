package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
)

// TestImportSkillsBlockedAfterUnresolvedIntent proves the
// recovery-blocks-writes invariant inside a begun batch: once an item
// leaves an unresolved Store operation intent (Restore could not prove the
// state), no later item performs any Store mutation; the remaining
// selections are reported failed with CodeRecovery and their content is
// never materialized or installed.
func TestImportSkillsBlockedAfterUnresolvedIntent(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/a": "A", "skills/b": "B", "skills/c": "C"})
	ctx, cancel := context.WithCancel(context.Background())
	materialized := 0
	a.materialize = func(c context.Context, loc source.Locator, _ string, entry source.Entry, workDir, dst string, allowLarge bool) (string, error) {
		materialized++
		return source.MaterializeEntry(c, loc, src.LastCommit, entry, workDir, dst, allowLarge)
	}
	n := 0
	a.commitHook = func(afterCommit bool) {
		if afterCommit {
			return
		}
		n++
		if n == 2 {
			cancel()
			// Tamper item b's installed live tree so Restore cannot prove
			// it as this operation's install and leaves an unresolved
			// pending intent.
			if err := os.WriteFile(filepath.Join(a.StorePath, "b", "SKILL.md"), []byte("tampered"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	res, err := a.ImportSkills(ctx, ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatalf("an unresolved intent must not become a top-level error: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("every item must receive an outcome: %+v", res.Items)
	}
	if res.Items[0].Status != StatusImported {
		t.Fatalf("first item: %+v", res.Items[0])
	}
	if res.Items[1].Status != StatusFailed || res.Items[1].ErrorCode != CodeRecovery {
		t.Fatalf("item leaving the unresolved intent: %+v", res.Items[1])
	}
	if res.Items[2].Status != StatusFailed || res.Items[2].ErrorCode != CodeRecovery {
		t.Fatalf("blocked sibling: %+v", res.Items[2])
	}
	if materialized != 2 {
		t.Fatalf("materializations: %d, want 2 (blocked sibling must not materialize)", materialized)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "c")); !os.IsNotExist(err) {
		t.Fatal("blocked sibling must not be installed")
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "b", "SKILL.md")); err != nil || string(data) != "tampered" {
		t.Fatalf("unprovable live content must be preserved: %q, %v", data, err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Kind != skillstore.KindImport || ops[0].Slug != "b" {
		t.Fatalf("the unresolved intent must remain: %+v", ops)
	}
}

// TestImportSkillsUnresolvedCommittedIntentBlocksSiblings proves a
// committed item whose finalization cannot complete (the live tree was
// tampered after the commit) leaves a committed intent that blocks every
// later Store write in the batch; the item still reports its Skill id and
// slug.
func TestImportSkillsUnresolvedCommittedIntentBlocksSiblings(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/a": "A", "skills/b": "B"})
	n := 0
	a.commitHook = func(afterCommit bool) {
		if afterCommit && n == 0 {
			n++
			if err := os.WriteFile(filepath.Join(a.StorePath, "a", "SKILL.md"), []byte("tampered"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatalf("unresolved committed intent must not become a top-level error: %v", err)
	}
	if res.Items[0].Status != StatusFailed || res.Items[0].ErrorCode != CodeRecovery ||
		res.Items[0].SkillID == 0 || res.Items[0].Slug != "a" {
		t.Fatalf("committed item: %+v", res.Items[0])
	}
	if res.Items[1].Status != StatusFailed || res.Items[1].ErrorCode != CodeRecovery {
		t.Fatalf("blocked sibling: %+v", res.Items[1])
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "b")); !os.IsNotExist(err) {
		t.Fatal("blocked sibling must not be installed")
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "a", "SKILL.md")); err != nil || string(data) != "tampered" {
		t.Fatalf("tampered content must be preserved: %q, %v", data, err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("the committed intent must remain: %+v", ops)
	}
}

// TestImportSkillsDeleteFailureAfterFinalizeIsRecovery proves a committed
// item whose terminal row cannot be deleted after a successful finalize is
// reported failed/recovery_failed with its Skill id and slug; the terminal
// finalized row (with its durable receipt) is retained and the batch is
// Store-blocked. The terminal Store state was completed and the receipt
// was persisted before the SQL deletion failed, so the next run converges:
// it validates the receipt-bound terminal result and CAS-deletes the row.
func TestImportSkillsDeleteFailureAfterFinalizeIsRecovery(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	a.deleteOperation = func(int64) error { return errors.New("delete failed") }

	res := importSkills(t, a, src.ID, "skills/alpha")
	item := res.Items[0]
	if item.Status != StatusFailed || item.ErrorCode != CodeRecovery ||
		item.Slug != "alpha" || item.RequestedSlug != "alpha" || item.SkillID == 0 {
		t.Fatalf("finalized-but-uncleared item: %+v", item)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseFinalized || len(ops[0].Receipt) == 0 {
		t.Fatalf("the terminal finalized intent must remain with its receipt: %+v", ops)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(item.SkillID), "SKILL.md")); err != nil {
		t.Fatalf("finalize must have completed: %v", err)
	}

	// The terminal Store state was completed and the durable receipt was
	// persisted before the SQL deletion failed: the next run validates the
	// receipt-bound terminal result and CAS-deletes the terminal row, then
	// imports the already-committed entry as a no-op.
	a.deleteOperation = nil
	again := importSkills(t, a, src.ID, "skills/alpha")
	if again.Items[0].Status != StatusAlreadyImported || again.Items[0].SkillID != item.SkillID {
		t.Fatalf("import after terminal convergence: %+v", again.Items[0])
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("the terminal row must be cleared: %+v", ops)
	}
}

// TestImportSkillsDeleteFailureAfterRestoreIsRecovery proves a cancelled
// item whose terminal restore succeeded but whose terminal-row deletion
// failed is reported failed/recovery_failed, leaves a residual restored row
// with its durable receipt, and blocks later Store writes; the next run
// validates the receipt-bound restored result and CAS-deletes the row, then
// imports both entries.
func TestImportSkillsDeleteFailureAfterRestoreIsRecovery(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/a": "A", "skills/b": "B"})
	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			cancel()
		}
	}
	a.deleteOperation = func(int64) error { return errors.New("delete failed") }

	res, err := a.ImportSkills(ctx, ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatalf("cleanup failure must not become a top-level error: %v", err)
	}
	if res.Items[0].Status != StatusFailed || res.Items[0].ErrorCode != CodeRecovery ||
		res.Items[0].Slug != "a" || res.Items[0].RequestedSlug != "a" {
		t.Fatalf("restored-but-uncleared item: %+v", res.Items[0])
	}
	if res.Items[1].Status != StatusFailed || res.Items[1].ErrorCode != CodeRecovery {
		t.Fatalf("blocked sibling: %+v", res.Items[1])
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "a")); !os.IsNotExist(err) {
		t.Fatal("restore must have removed the installed live tree")
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseRestored || len(ops[0].Receipt) == 0 {
		t.Fatalf("residual restored intent with its receipt: %+v", ops)
	}

	// the next batch validates the receipt-bound restored result, clears
	// the terminal row, and imports both entries
	a.deleteOperation = nil
	again, err := a.ImportSkills(context.Background(), ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if again.Items[0].Status != StatusImported || again.Items[1].Status != StatusImported {
		t.Fatalf("after terminal convergence: %+v", again.Items)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("intents must be cleared: %+v", ops)
	}
}
