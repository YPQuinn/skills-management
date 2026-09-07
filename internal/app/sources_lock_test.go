package app

import (
	"context"
	"errors"
	"testing"

	"skillctl/internal/lock"
	"skillctl/internal/source"
)

func TestCheckSourceCancellationLeavesState(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root})
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.observer = cancelObserver{cancel: cancel}
	if _, err := a.CheckSource(ctx, src.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled check: %v", err)
	}
	after, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}

	if before.LastCheckedAt == nil || after.LastCheckedAt == nil || !before.LastCheckedAt.Equal(*after.LastCheckedAt) {
		t.Fatalf("cancelled check must not advance LastCheckedAt: before %v, after %v", before.LastCheckedAt, after.LastCheckedAt)
	}
	if before.LastCheckStartedAt == nil || after.LastCheckStartedAt == nil || !before.LastCheckStartedAt.Equal(*after.LastCheckStartedAt) {
		t.Fatalf("cancelled check must not advance LastCheckStartedAt")
	}
	if before.LastCheckResult != after.LastCheckResult {
		t.Fatalf("cancelled check must not change the check result: %q -> %q", before.LastCheckResult, after.LastCheckResult)
	}
	if before.Available != after.Available || len(before.Entries) != len(after.Entries) {
		t.Fatalf("cancelled check must not change availability or Inventory: before %+v, after %+v", before, after)
	}
}

func TestLockContentionMapsToCodeLocked(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root})
	if err != nil {
		t.Fatal(err)
	}

	// observer-level lock contention (for example the Git cache lock) is
	// CodeLocked and never mutates Source availability.
	a.observer = fakeObserver{err: lock.ErrLocked}
	if _, err := a.CheckSource(context.Background(), src.ID); !isCode(err, CodeLocked) {
		t.Fatalf("check under observer lock contention: %v", err)
	}
	shown, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !shown.Available || shown.LastError != "" {
		t.Fatalf("lock contention must not mutate availability: %+v", shown)
	}

	// registration under lock contention is CodeLocked and saves nothing
	other := t.TempDir()
	writeSourceSkill(t, other, "beta")
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: other}); !isCode(err, CodeLocked) {
		t.Fatalf("add under lock contention: %v", err)
	}
	items, err := a.ListSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("lock contention must not save a Source: %+v", items)
	}
}

func TestCheckSourcePerSourceLock(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root})
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}

	lockPath, err := a.sourceLockPath(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Unlock()

	if _, err := a.CheckSource(context.Background(), src.ID); !isCode(err, CodeLocked) {
		t.Fatalf("check under the per-Source lock: %v", err)
	}
	after, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.LastCheckedAt == nil || after.LastCheckedAt == nil || !before.LastCheckedAt.Equal(*after.LastCheckedAt) {
		t.Fatalf("lock contention must not advance LastCheckedAt: before %v, after %v", before.LastCheckedAt, after.LastCheckedAt)
	}
	if !after.Available {
		t.Fatal("lock contention must not mark the Source unavailable")
	}
}

// TestCheckSourceLockBeforeRead proves the per-Source lock is acquired before
// the Source row is read: a check whose lock is held reports CodeLocked even
// for an id that does not exist, because the lock attempt precedes the state
// read. A delayed check can therefore never act on stale pre-lock state.
// Without contention the missing Source still reports not_found.
func TestCheckSourceLockBeforeRead(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	const missingID = 9999

	lockPath, err := a.sourceLockPath(missingID)
	if err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := a.CheckSource(context.Background(), missingID); !isCode(err, CodeLocked) {
		held.Unlock()
		t.Fatalf("check under a held lock must report CodeLocked before any state read: %v", err)
	}
	if err := held.Unlock(); err != nil {
		t.Fatal(err)
	}

	if _, err := a.CheckSource(context.Background(), missingID); !isCode(err, CodeNotFound) {
		t.Fatalf("uncontended check of a missing Source: %v", err)
	}
}
