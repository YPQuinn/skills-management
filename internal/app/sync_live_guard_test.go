package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/source"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// TestAutoSyncRefusesLocalEditDuringMaterialize locks the live-digest
// binding of the automatic source_changed replacement: the locked
// evaluation's live digest is the replace guard. A local edit landing
// between the evaluation and the replace is refused and reclassified as a
// conflict, never auto-overwritten.
func TestAutoSyncRefusesLocalEditDuringMaterialize(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")

	// The materialize seam edits the live Store tree while the Source
	// content is being materialized: exactly the local-edit window between
	// the locked evaluation and the replace.
	a.materialize = func(ctx context.Context, loc source.Locator, commit string, entry source.Entry, workDir, dst string, allowLarge bool) (string, error) {
		d, err := source.MaterializeEntry(ctx, loc, commit, entry, workDir, dst, allowLarge)
		if err == nil {
			rewriteStoreFile(t, a, "demo", "local.txt", "local edit\n")
		}
		return d, err
	}

	item, err := a.SyncSkill(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Result != sync.ResultSkipped || item.Status != sync.StatusConflict {
		t.Fatalf("the local edit must be reclassified as a conflict: %+v", item)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "local.txt")); err != nil || string(data) != "local edit\n" {
		t.Fatalf("the local edit must survive byte for byte: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the Source change must not be applied")
	}
	if d := skillDetailOf(t, a, skillID); d.Skill.SyncStatus != string(sync.StatusConflict) {
		t.Fatalf("the relationship must persist as conflict: %+v", d.Skill)
	}
	if _, err := state.GetSnapshot(a.db, skillID); err == nil {
		t.Fatal("a refused replace must not rotate a snapshot")
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("a refused replace must leave no open operation: %+v", ops)
	}

	// The next automatic sync still skips the conflict; the explicit
	// Accept Source then resolves it against the current live.
	a.materialize = source.MaterializeEntry
	again, err := a.SyncSkill(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Result != sync.ResultSkipped || again.Status != sync.StatusConflict {
		t.Fatalf("the follow-up sync must keep skipping: %+v", again)
	}
	accepted, err := a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Result != sync.ResultAcceptedSource || accepted.Status != sync.StatusInSync {
		t.Fatalf("explicit accept must resolve the conflict: %+v", accepted)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "notes.md")); err != nil || string(data) != "upstream\n" {
		t.Fatalf("the accepted Source content must be live: %q, %v", data, err)
	}
}

// TestAcceptSourceRefusesMidMaterializeEdit locks the explicit-acceptance
// guard: Accept Source replaces the live tree the confirmation sampled
// under the lock. A local edit landing during the materialization is
// refused as a conflict (never silently overwritten), and the retry
// re-confirms the current live and succeeds.
func TestAcceptSourceRefusesMidMaterializeEdit(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")

	edited := false
	a.materialize = func(ctx context.Context, loc source.Locator, commit string, entry source.Entry, workDir, dst string, allowLarge bool) (string, error) {
		d, err := source.MaterializeEntry(ctx, loc, commit, entry, workDir, dst, allowLarge)
		if err == nil && !edited {
			edited = true
			rewriteStoreFile(t, a, "demo", "local.txt", "mid-flight edit\n")
		}
		return d, err
	}

	item, err := a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Result != sync.ResultFailed || item.ErrorCode != CodeConflict {
		t.Fatalf("a mid-materialize edit must refuse the acceptance: %+v", item)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "local.txt")); err != nil || string(data) != "mid-flight edit\n" {
		t.Fatalf("the mid-flight edit must survive: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the Source change must not be applied")
	}
	if _, err := state.GetSnapshot(a.db, skillID); err == nil {
		t.Fatal("a refused accept must not rotate a snapshot")
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("a refused accept must leave no open operation: %+v", ops)
	}

	a.materialize = source.MaterializeEntry
	accepted, err := a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Result != sync.ResultAcceptedSource || accepted.Status != sync.StatusInSync {
		t.Fatalf("the retried accept must succeed: %+v", accepted)
	}
	// The displaced confirmation live (with the mid-flight edit) is the
	// new previous snapshot.
	snap, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != accepted.BeforeDigest || snap.Reason != "accept_source" {
		t.Fatalf("snapshot after accept: %+v", snap)
	}
}

// TestRollbackUnboundSkillSucceeds locks the Binding-independent rollback:
// a Skill whose Source Binding was removed still rolls back its previous
// snapshot, and the rotated snapshot records empty Source evidence.
func TestRollbackUnboundSkillSucceeds(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	if r, err := a.SyncSkill(context.Background(), skillID); err != nil || r.Result != sync.ResultUpdated {
		t.Fatalf("sync: %+v, %v", r, err)
	}
	// The detach use case arrives later; the Store state must already
	// tolerate a removed Binding with a surviving snapshot.
	if _, err := a.db.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, skillID); err != nil {
		t.Fatal(err)
	}

	r, err := a.Rollback(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultRolledBack || r.Status != sync.StatusUnbound {
		t.Fatalf("unbound rollback: %+v", r)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the pre-sync content must be live again: %v", err)
	}
	snap, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Reason != "rollback" || snap.SourceCommit != "" {
		t.Fatalf("the unbound rollback snapshot must carry empty Source evidence: %+v", snap)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no open operations: %+v", ops)
	}
}
