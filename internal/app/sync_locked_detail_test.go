package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"skillctl/internal/source"
	syncpkg "skillctl/internal/sync"
)

// firstObserveGate blocks the first observation after it completed but
// before its caller continues (the per-Source lock is still held), so a
// test can deterministically interleave another action between a
// synchronization's outside samples and its Store lock acquisition.
type firstObserveGate struct {
	source.Observer
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (g *firstObserveGate) Observe(ctx context.Context, loc source.Locator, workDir string) (source.Observation, error) {
	obs, err := g.Observer.Observe(ctx, loc, workDir)
	g.once.Do(func() {
		close(g.entered)
		<-g.release
	})
	return obs, err
}

func waitFor(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the gated observation")
	}
}

// TestSyncSkillReReadsBaselineUnderLock locks the lock-scoped Skill facts:
// while a sync waits between its fresh observation and the Store lock, a
// concurrent Keep Store advances the Baseline to the same Source content.
// The sync must re-read the Skill under the lock and reclassify
// store_changed (skip), never act on the stale outside sample that would
// classify source_changed and auto-overwrite the Store.
func TestSyncSkillReReadsBaselineUnderLock(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	src = checkSourceNow(t, a, src)
	dir, digest := materializeTo(t, a, src, "skills/demo")

	gate := &firstObserveGate{Observer: a.observer, entered: make(chan struct{}), release: make(chan struct{})}
	a.observer = gate

	type outcome struct {
		item *SyncItemResult
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		item, err := a.SyncSkill(context.Background(), skillID)
		done <- outcome{item, err}
	}()

	waitFor(t, gate.entered)
	// While the sync waits for the Store lock, another action advances
	// the Baseline to the Source content through the durable journal.
	if err := a.withStoreLock(context.Background(), func() error {
		fail := a.baselineRefresh(context.Background(), skillID, src, dir, digest)
		if fail.code != "" {
			t.Fatalf("concurrent Baseline refresh: %+v", fail)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	close(gate.release)

	out := <-done
	if out.err != nil {
		t.Fatal(out.err)
	}
	if out.item.Result != syncpkg.ResultSkipped || out.item.Status != syncpkg.StatusStoreChanged {
		t.Fatalf("the waiting sync must re-evaluate the advanced Baseline: %+v", out.item)
	}
	// The stale source_changed classification must never overwrite the
	// Store after the concurrent acceptance.
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the Store must stay untouched: %v", err)
	}
	if d := skillDetailOf(t, a, skillID); d.Skill.BaselineDigest != digest {
		t.Fatalf("baseline: %q, want %q", d.Skill.BaselineDigest, digest)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no open operations: %+v", ops)
	}
}

// TestSyncBatchReReadsDetailsUnderLock locks the same rule for the batch:
// the Binding list is sampled before the observation, and each item's facts
// are re-read under the held lock, so a Baseline advanced by a concurrent
// action while the batch waited reclassifies that item instead of
// overwriting it, while the sibling item still updates from the same fresh
// observation.
func TestSyncBatchReReadsDetailsUnderLock(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha", "skills/beta": "Beta"})
	if res := importSkills(t, a, src.ID, "skills/alpha"); res.Items[0].Status != StatusImported {
		t.Fatalf("import alpha: %+v", res.Items)
	}
	if res := importSkills(t, a, src.ID, "skills/beta"); res.Items[0].Status != StatusImported {
		t.Fatalf("import beta: %+v", res.Items)
	}
	alphaID := mustSkillID(t, a, "alpha")
	rewriteSourceFile(t, src, "skills/alpha", "notes.md", "alpha v2\n")
	rewriteSourceFile(t, src, "skills/beta", "notes.md", "beta v2\n")
	src = checkSourceNow(t, a, src)
	dir, digest := materializeTo(t, a, src, "skills/alpha")

	gate := &firstObserveGate{Observer: a.observer, entered: make(chan struct{}), release: make(chan struct{})}
	a.observer = gate

	type outcome struct {
		result *SyncSkillsResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := a.SyncSkills(context.Background(), src.ID)
		done <- outcome{result, err}
	}()

	waitFor(t, gate.entered)
	if err := a.withStoreLock(context.Background(), func() error {
		fail := a.baselineRefresh(context.Background(), alphaID, src, dir, digest)
		if fail.code != "" {
			t.Fatalf("concurrent Baseline refresh: %+v", fail)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	close(gate.release)

	out := <-done
	if out.err != nil {
		t.Fatal(out.err)
	}
	bySlug := map[string]SyncItemResult{}
	for _, it := range out.result.Items {
		bySlug[it.Slug] = it
	}
	if bySlug["alpha"].Result != syncpkg.ResultSkipped || bySlug["alpha"].Status != syncpkg.StatusStoreChanged {
		t.Fatalf("alpha must re-evaluate the advanced Baseline: %+v", bySlug["alpha"])
	}
	if bySlug["beta"].Result != syncpkg.ResultUpdated {
		t.Fatalf("beta must update from the shared fresh observation: %+v", bySlug["beta"])
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha", "notes.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("alpha must stay untouched: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "beta", "notes.md")); err != nil || string(data) != "beta v2\n" {
		t.Fatalf("beta content: %q, %v", data, err)
	}
}

// TestAcceptSourceRefusesBindingChangeUnderLock locks the Binding check:
// while an explicit Accept Source waits for the Store lock, the Binding is
// removed. The acceptance must refuse with conflict instead of replacing
// the Store through the stale outside facts.
func TestAcceptSourceRefusesBindingChangeUnderLock(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")

	gate := &firstObserveGate{Observer: a.observer, entered: make(chan struct{}), release: make(chan struct{})}
	a.observer = gate

	type outcome struct {
		item *SyncItemResult
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		item, err := a.AcceptSource(context.Background(), skillID)
		done <- outcome{item, err}
	}()

	waitFor(t, gate.entered)
	if _, err := a.db.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, skillID); err != nil {
		t.Fatal(err)
	}
	close(gate.release)

	out := <-done
	if !isCode(out.err, CodeConflict) {
		t.Fatalf("an unbound Skill must refuse Accept Source with conflict: %+v, %v", out.item, out.err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the Store must stay untouched: %v", err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("no open operations: %+v", ops)
	}
}

// mustSkillID resolves one Skill by slug for a test.
func mustSkillID(t *testing.T, a *App, slug string) int64 {
	t.Helper()
	id, err := a.ResolveSkillArg(slug)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
