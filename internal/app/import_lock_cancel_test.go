package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/lock"
	"skillctl/internal/source"
)

// TestImportSkillsSourceLockContention proves the per-Source lock is taken
// before observation and its contention is CodeLocked with no Store writes.
func TestImportSkillsSourceLockContention(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	lockPath, err := a.sourceLockPath(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Unlock()

	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !isCode(err, CodeLocked) {
		t.Fatalf("source lock contention: %v", err)
	}
	if skills, err := a.ListSkills(); err != nil || len(skills) != 0 {
		t.Fatalf("no import may proceed under lock contention: %+v, %v", skills, err)
	}
}

// TestImportSkillsStoreLockContention proves a second Store writer receives
// CodeLocked while the exclusive Store lock is held.
func TestImportSkillsStoreLockContention(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	lockPath, err := a.storeLockPath()
	if err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Unlock()

	if _, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !isCode(err, CodeLocked) {
		t.Fatalf("store lock contention: %v", err)
	}
	if skills, err := a.ListSkills(); err != nil || len(skills) != 0 {
		t.Fatalf("no import may proceed under lock contention: %+v, %v", skills, err)
	}
}

// TestImportSkillsUnavailableSourcePersistsFailedCheck proves an
// unobservable Source records the same failed check metadata as CheckSource
// and blocks the batch before any Store mutation.
func TestImportSkillsUnavailableSourcePersistsFailedCheck(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	if err := os.Rename(src.Location, src.Location+"-moved"); err != nil {
		t.Fatal(err)
	}

	_, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeSourceUnavailable) {
		t.Fatalf("unavailable source: %v", err)
	}
	shown, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Available || shown.LastError == "" || len(shown.Entries) != 1 {
		t.Fatalf("failed check metadata must be persisted: %+v", shown)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); !os.IsNotExist(err) {
		t.Fatal("the Store must be untouched")
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no intent may exist: %+v", ops)
	}
}

// TestImportSkillsCancelledBeforeProcessing proves a pre-cancelled request
// fails before any Store work with no item outcomes.
func TestImportSkillsCancelledBeforeProcessing(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := a.ImportSkills(ctx, ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled batch: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); !os.IsNotExist(err) {
		t.Fatal("the Store must be untouched")
	}
}

// TestImportSkillsCancelledMidMaterialize proves cancellation inside item
// processing never becomes a top-level error and every item still receives
// an outcome; the cancelled item leaves no live tree or intent.
func TestImportSkillsCancelledMidMaterialize(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/a": "A", "skills/b": "B", "skills/c": "C"})
	ctx, cancel := context.WithCancel(context.Background())
	a.materialize = func(c context.Context, loc source.Locator, _ string, entry source.Entry, workDir, dst string, allowLarge bool) (string, error) {
		if entry.RelativeDir == "skills/b" {
			cancel()
			return "", errors.New("cancelled mid-materialize")
		}
		return source.MaterializeEntry(c, loc, src.LastCommit, entry, workDir, dst, allowLarge)
	}

	res, err := a.ImportSkills(ctx, ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatalf("cancellation during the batch must not become a top-level error: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("every item must receive an outcome: %+v", res.Items)
	}
	if res.Items[0].Status != StatusImported || res.Items[0].Slug != "a" {
		t.Fatalf("first item: %+v", res.Items[0])
	}
	if res.Items[1].Status != StatusFailed || res.Items[1].ErrorCode != CodeCancelled {
		t.Fatalf("cancelled item: %+v", res.Items[1])
	}
	if res.Items[2].Status != StatusFailed || res.Items[2].ErrorCode != CodeCancelled {
		t.Fatalf("later item under cancellation: %+v", res.Items[2])
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "b")); !os.IsNotExist(err) {
		t.Fatal("cancelled item must not leave a live tree")
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("cancelled item must not leave an intent: %+v", ops)
	}
}

// TestImportSkillsCancelledAfterInstallBeforeCommit proves cancellation
// between Install and the SQLite commit unwinds the pending operation
// deterministically: the installed live tree is removed and the intent is
// cleared, while already-committed siblings stay intact.
func TestImportSkillsCancelledAfterInstallBeforeCommit(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/a": "A", "skills/b": "B", "skills/c": "C"})
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			n++
			if n == 2 {
				cancel()
			}
		}
	}

	res, err := a.ImportSkills(ctx, ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatalf("cancellation before commit must not become a top-level error: %v", err)
	}
	if res.Items[0].Status != StatusImported || res.Items[1].Status != StatusFailed ||
		res.Items[1].ErrorCode != CodeCancelled || res.Items[2].Status != StatusFailed {
		t.Fatalf("outcomes: %+v", res.Items)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "a")); err != nil {
		t.Fatalf("committed sibling must survive: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "b")); !os.IsNotExist(err) {
		t.Fatal("cancelled item's install must be restored")
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no intent may survive: %+v", ops)
	}
}

// TestImportSkillsCancelledAfterCommitFinishesFinalize proves that once the
// SQLite commit has completed, cancellation cannot interrupt finalization:
// the item still reports imported with its Baseline installed.
func TestImportSkillsCancelledAfterCommitFinishesFinalize(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if afterCommit {
			cancel()
		}
	}

	res, err := a.ImportSkills(ctx, ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := res.Items[0]
	if item.Status != StatusImported || item.SkillID == 0 {
		t.Fatalf("committed item must finalize: %+v", item)
	}
	baseline := filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(item.SkillID))
	if _, err := os.Lstat(filepath.Join(baseline, "SKILL.md")); err != nil {
		t.Fatalf("finalization must complete despite cancellation: %v", err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("intent must be cleared: %+v", ops)
	}
}
