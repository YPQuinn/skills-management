package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/sync"
)

// swapObserver wraps an observer and mutates content after the scan
// returns: the concurrent change lands between the fresh observation and
// the Store-locked confirmation.
type swapObserver struct {
	source.Observer
	swap func()
}

func (o swapObserver) Observe(ctx context.Context, loc source.Locator, workDir string) (source.Observation, error) {
	obs, err := o.Observer.Observe(ctx, loc, workDir)
	if err == nil {
		o.swap()
	}
	return obs, err
}

// convergeFixture builds the independent-convergence state: the Store
// diverges first, then the Source moves to exactly the same content, so a
// check observes Source == Store while the Baseline lags.
func convergeFixture(t *testing.T, a *App) (*source.Source, int64) {
	t.Helper()
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteStoreFile(t, a, "demo", "SKILL.md", "---\nname: demo\ndescription: desc\n---\n# locally edited\n")
	data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(src.Location, "skills", "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src.Location, "skills", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src.Location, "skills", "demo", "SKILL.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return src, skillID
}

// TestKeepStoreCancelBeforeCommit locks the pre-commit failure of the
// Baseline-only journal: a cancellation exactly before the SQLite commit
// discards only the operation's own staging and leaves the Baseline tree,
// the persisted Baseline digest, the Binding, and the live tree exactly as
// they were, with no open operation.
func TestKeepStoreCancelBeforeCommit(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	pre := skillDetailOf(t, a, skillID)
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")

	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			cancel()
		}
	}
	r, err := a.KeepStore(ctx, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultFailed || r.ErrorCode != CodeCancelled {
		t.Fatalf("cancelled keep-store: %+v", r)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest != pre.Skill.BaselineDigest || d.Binding.Digest != pre.Binding.Digest {
		t.Fatalf("failed keep-store must not touch Baseline or Binding: %+v -> %+v", pre, d)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID), "SKILL.md")); err != nil ||
		!strings.Contains(string(data), "# demo") {
		t.Fatalf("baseline tree mutated: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("live tree mutated: %v", err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("the failed keep-store must leave no open operation: %+v", ops)
	}
}

// TestKeepStoreBlockedFinalizeConverges locks the post-commit failure of
// the Baseline-only journal: when the durable receipt CAS fails exactly
// after the semantic finalize, the committed Baseline is already in effect,
// the committed row stays open, and the next Store write converges through
// recovery instead of re-mutating the Baseline.
func TestKeepStoreBlockedFinalizeConverges(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")

	a.receiptPersist = func(op skillstore.Operation) error { return os.ErrInvalid }
	r, err := a.KeepStore(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultFailed || r.ErrorCode != CodeRecovery {
		t.Fatalf("blocked keep-store: %+v", r)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest == "" || d.Binding.Digest != d.Skill.BaselineDigest {
		t.Fatalf("committed Baseline must be in effect: %+v", d)
	}
	if ops := openOperations(t, a); len(ops) != 1 {
		t.Fatalf("the unresolved operation must stay open: %+v", ops)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID), "notes.md")); err != nil ||
		string(data) != "upstream\n" {
		t.Fatalf("baseline tree after commit: %q, %v", data, err)
	}
	// The live content was never touched.
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("live tree mutated: %v", err)
	}

	// The next write recovers the committed refresh idempotently: a check
	// always takes the Store lock, and the recovered Baseline makes the
	// relationship store_changed (Store equals the old content).
	a.receiptPersist = nil
	checked, err := a.CheckSkillSync(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusStoreChanged) {
		t.Fatalf("converged check: %+v", checked)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("recovery must clear the open operation: %+v", ops)
	}
}

// TestCheckSkillSyncConfirmsConvergenceUnderLock locks the in-lock
// confirmation seam: a concurrent Store edit that lands between the fresh
// observation and the locked evaluation must never be reported in_sync and
// must never advance the Baseline, because the Skill facts, the Source
// facts, and the live tree are all re-sampled under the Store lock.
func TestCheckSkillSyncConfirmsConvergenceUnderLock(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	_, skillID := convergeFixture(t, a)
	preBaseline := skillDetailOf(t, a, skillID).Skill.BaselineDigest
	a.observer = swapObserver{a.observer, func() {
		rewriteStoreFile(t, a, "demo", "sneaky.txt", "external edit\n")
	}}
	checked, err := a.CheckSkillSync(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusConflict) {
		t.Fatalf("a concurrent Store edit must not report in_sync: %+v", checked)
	}
	if d := skillDetailOf(t, a, skillID); d.Skill.BaselineDigest != preBaseline {
		t.Fatalf("the Baseline must not advance on a stale outside sample: %+v", d)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no journal may exist: %+v", ops)
	}
}

// TestCheckSkillSyncRefusesSourceChangedDuringCheck locks the materialize
// seam: when the Source content changes after the observation but before
// the locked materialization, the check fails with conflict instead of
// advancing the Baseline to content the Source no longer carries.
func TestCheckSkillSyncRefusesSourceChangedDuringCheck(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := convergeFixture(t, a)
	preBaseline := skillDetailOf(t, a, skillID).Skill.BaselineDigest
	a.observer = swapObserver{a.observer, func() {
		rewriteSourceFile(t, src, "skills/demo", "SKILL.md", "---\nname: demo\ndescription: desc\n---\n# changed under the check\n")
	}}
	_, err := a.CheckSkillSync(context.Background(), skillID)
	if err == nil || errorCodeOf(err) != CodeConflict {
		t.Fatalf("a Source change during the check must fail with conflict: %v", err)
	}
	if d := skillDetailOf(t, a, skillID); d.Skill.BaselineDigest != preBaseline {
		t.Fatalf("the Baseline must not advance: %+v", d)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no journal may exist: %+v", ops)
	}
}

// TestConvergenceBlockedFinalizeConverges locks the post-commit failure of
// the convergence path: the committed Baseline advance survives a failed
// terminal row deletion and the next check converges through recovery.
func TestConvergenceBlockedFinalizeConverges(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	_, skillID := convergeFixture(t, a)
	liveDigest, missing, invalid := a.storeTreeState(context.Background(), "demo")
	if missing || invalid {
		t.Fatalf("live store state: %q %v %v", liveDigest, missing, invalid)
	}
	a.deleteOperation = func(int64) error { return os.ErrInvalid }
	_, err := a.CheckSkillSync(context.Background(), skillID)
	if err == nil || errorCodeOf(err) != CodeRecovery {
		t.Fatalf("blocked convergence check: %v", err)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest != liveDigest || d.Binding.Digest != liveDigest {
		t.Fatalf("committed convergence must be in effect: %+v", d)
	}
	if ops := openOperations(t, a); len(ops) != 1 {
		t.Fatalf("the unresolved operation must stay open: %+v", ops)
	}
	a.deleteOperation = nil
	checked, err := a.CheckSkillSync(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusInSync) || checked.SyncStale {
		t.Fatalf("converged check: %+v", checked)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("recovery must clear the open operation: %+v", ops)
	}
}

// TestAcceptSourceRepairCancelBeforeCommit locks the pre-commit failure of
// the repair path: the repair import with the journaled Baseline advance
// unwinds the installed tree and leaves the persisted Baseline and its
// tree exactly as they were.
func TestAcceptSourceRepairCancelBeforeCommit(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	_, skillID := importOneSkill(t, a, "skills/demo")
	preBaseline := skillDetailOf(t, a, skillID).Skill.BaselineDigest
	if err := os.RemoveAll(filepath.Join(a.StorePath, "demo")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			cancel()
		}
	}
	r, err := a.AcceptSource(ctx, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultFailed || r.ErrorCode != CodeCancelled {
		t.Fatalf("cancelled repair: %+v", r)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest != preBaseline {
		t.Fatalf("failed repair must not touch the Baseline: %+v", d)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo")); !os.IsNotExist(err) {
		t.Fatalf("the repair must unwind the installed tree: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID), "SKILL.md")); err != nil ||
		!strings.Contains(string(data), "# demo") {
		t.Fatalf("baseline tree mutated: %q, %v", data, err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("the failed repair must leave no open operation: %+v", ops)
	}
}

// TestAcceptSourceRepairBlockedFinalizeConverges locks the post-commit
// failure of the repair path: the committed import advances the Baseline in
// the same journal, the failed terminal row deletion keeps the operation
// open, and the next Accept Source converges through recovery.
func TestAcceptSourceRepairBlockedFinalizeConverges(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	_, skillID := importOneSkill(t, a, "skills/demo")
	if err := os.RemoveAll(filepath.Join(a.StorePath, "demo")); err != nil {
		t.Fatal(err)
	}
	a.deleteOperation = func(int64) error { return os.ErrInvalid }
	r, err := a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultFailed || r.ErrorCode != CodeRecovery {
		t.Fatalf("blocked repair: %+v", r)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest == "" || d.Skill.BaselineDigest != d.Skill.StoreDigest || d.Binding.Digest != d.Skill.StoreDigest {
		t.Fatalf("committed repair: %+v", d)
	}
	if ops := openOperations(t, a); len(ops) != 1 {
		t.Fatalf("the unresolved operation must stay open: %+v", ops)
	}
	a.deleteOperation = nil
	r, err = a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultNoOp || r.Status != sync.StatusInSync {
		t.Fatalf("converged repair: %+v", r)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("recovery must clear the open operation: %+v", ops)
	}
}

// TestRollbackEvaluatesPersistedSourceFacts locks the post-rollback
// evaluation against the latest persisted Source inventory, issues, and
// availability — never the Binding digest, which records the accepted
// content and would mask a Source that moved or lost the entry.
func TestRollbackEvaluatesPersistedSourceFacts(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	if r, err := a.SyncSkill(context.Background(), skillID); err != nil || r.Result != sync.ResultUpdated {
		t.Fatalf("sync: %+v, %v", r, err)
	}
	// The Source moves on without a sync: the persisted inventory now
	// differs from the Baseline and from the Store, so a rollback recomputes
	// a conflict instead of silently treating the Binding digest as the
	// current Source.
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream 2\n")
	if checked, err := a.CheckSkillSync(context.Background(), skillID); err != nil || checked.SyncStatus != string(sync.StatusSourceChanged) {
		t.Fatalf("pre-state: %+v, %v", checked, err)
	}
	rolled, err := a.Rollback(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Result != sync.ResultRolledBack || rolled.Status != sync.StatusConflict {
		t.Fatalf("rollback must evaluate against the persisted Source inventory: %+v", rolled)
	}

	// The persisted inventory loses the entry: the rollback reports
	// source_missing instead of silently recomputing from the Binding.
	if err := os.RemoveAll(filepath.Join(src.Location, "skills", "demo")); err != nil {
		t.Fatal(err)
	}
	if checked, err := a.CheckSkillSync(context.Background(), skillID); err != nil || checked.SyncStatus != string(sync.StatusSourceMissing) {
		t.Fatalf("missing entry: %+v, %v", checked, err)
	}
	rolled, err = a.Rollback(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Result != sync.ResultRolledBack || rolled.Status != sync.StatusSourceMissing || rolled.Stale {
		t.Fatalf("rollback after entry loss: %+v", rolled)
	}

	// An unreachable Source retains the last relationship marked stale.
	if err := os.RemoveAll(src.Location); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CheckSkillSync(context.Background(), skillID); err != nil {
		t.Fatal(err)
	}
	rolled, err = a.Rollback(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Result != sync.ResultRolledBack || !rolled.Stale || rolled.Status != sync.StatusSourceMissing {
		t.Fatalf("rollback with unreachable Source: %+v", rolled)
	}
}
