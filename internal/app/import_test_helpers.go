package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// writeSkillFile writes one Inventory entry at root/relDir with the given
// frontmatter name and a distinct body.
func writeSkillFile(t *testing.T, root, relDir, name string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: desc\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sourceAddInput is a shorthand Local Source registration input.
func sourceAddInput(root string) source.AddInput {
	return source.AddInput{Kind: source.KindLocal, Location: root}
}

// itoa formats an int64 without the strconv import at every call site.
func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

// addLocalSource registers a Local Source with one Inventory entry per
// (relDir, name) pair.
func addLocalSource(t *testing.T, a *App, entries map[string]string) *source.Source {
	t.Helper()
	root := t.TempDir()
	for relDir, name := range entries {
		writeSkillFile(t, root, relDir, name)
	}
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root})
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// countingObserver counts the number of Observe calls made through it.
type countingObserver struct {
	source.Observer
	n *int
}

func (c countingObserver) Observe(ctx context.Context, loc source.Locator, workDir string) (source.Observation, error) {
	*c.n++
	return c.Observer.Observe(ctx, loc, workDir)
}

func (c countingObserver) ObserveListing(ctx context.Context, loc source.Locator, workDir string) (source.Observation, error) {
	*c.n++
	if lo, ok := c.Observer.(listingObserver); ok {
		return lo.ObserveListing(ctx, loc, workDir)
	}
	return c.Observer.Observe(ctx, loc, workDir)
}

// findEntry returns the Inventory entry at relDir.
func findEntry(t *testing.T, src *source.Source, relDir string) source.Entry {
	t.Helper()
	for _, e := range src.Entries {
		if e.RelativeDir == relDir {
			return e
		}
	}
	t.Fatalf("entry %q not found in %+v", relDir, src.Entries)
	return source.Entry{}
}

// materializeTo copies one entry into a fresh temp dir with the real
// materializer, returning the dir and its canonical digest.
func materializeTo(t *testing.T, a *App, src *source.Source, relDir string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	digest, err := source.MaterializeEntry(context.Background(), src.Locator, src.LastCommit,
		findEntry(t, src, relDir), a.workDir(), dir, false)
	if err != nil {
		t.Fatal(err)
	}
	return dir, digest
}

// replaceAndCommit stages, installs, and SQLite-commits one replace,
// returning the committed operation before any finalization.
func replaceAndCommit(t *testing.T, a *App, src *source.Source, relDir, slug string) skillstore.Operation {
	t.Helper()
	dir, digest := materializeTo(t, a, src, relDir)
	detail, err := state.GetSkillDetailBySlug(a.db, slug)
	if err != nil {
		t.Fatal(err)
	}
	op := skillstore.Operation{
		SkillID: detail.Skill.ID, Slug: detail.Skill.Slug, Kind: skillstore.KindReplace,
		OldDigest: detail.Skill.StoreDigest, NewDigest: digest,
	}
	op, err = state.InsertOperation(a.db, op)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := a.store.Stage(context.Background(), op.ID, dir, digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.Install(context.Background(), op, &staged); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s := state.Skill{
		ID: detail.Skill.ID, Slug: detail.Skill.Slug, Name: detail.Skill.Name,
		Description: detail.Skill.Description, StoreDigest: digest, BaselineDigest: digest,
		CreatedAt: detail.Skill.CreatedAt, UpdatedAt: now,
	}
	b := state.Binding{SourceID: src.ID, RelativeDir: relDir, Digest: digest, SourceCommit: src.LastCommit, ImportedAt: now}
	if err := state.CommitReplace(a.db, s, b, op.ID); err != nil {
		t.Fatal(err)
	}
	op.SkillID = detail.Skill.ID
	return op
}

// importAndStopAfterStage persists a pending intent and stages the entry's
// content, leaving the live path untouched.
func importAndStopAfterStage(t *testing.T, a *App, src *source.Source, relDir, slug string) skillstore.Operation {
	t.Helper()
	dir, digest := materializeTo(t, a, src, relDir)
	op, err := state.InsertOperation(a.db, skillstore.Operation{Slug: slug, Kind: skillstore.KindImport, NewDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Stage(context.Background(), op.ID, dir, digest, false); err != nil {
		t.Fatal(err)
	}
	return op
}

// importAndStopBeforeCommit persists a pending intent and installs the live
// tree, leaving the SQLite commit undone.
func importAndStopBeforeCommit(t *testing.T, a *App, src *source.Source, relDir, slug string) skillstore.Operation {
	t.Helper()
	dir, digest := materializeTo(t, a, src, relDir)
	op, err := state.InsertOperation(a.db, skillstore.Operation{Slug: slug, Kind: skillstore.KindImport, NewDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	staged, err := a.store.Stage(context.Background(), op.ID, dir, digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.Install(context.Background(), op, &staged); err != nil {
		t.Fatal(err)
	}
	return op
}

// importAndStopBeforeFinalize runs the full durable sequence through the
// SQLite commit and leaves the committed intent for lazy recovery to
// finalize; it returns the committed operation.
func importAndStopBeforeFinalize(t *testing.T, a *App, src *source.Source, relDir, slug string) skillstore.Operation {
	t.Helper()
	dir, digest := materializeTo(t, a, src, relDir)
	op, err := state.InsertOperation(a.db, skillstore.Operation{Slug: slug, Kind: skillstore.KindImport, NewDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	staged, err := a.store.Stage(context.Background(), op.ID, dir, digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.Install(context.Background(), op, &staged); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	id, err := state.CommitImport(a.db, state.Skill{
		Slug: slug, Name: findEntry(t, src, relDir).Name, Description: "desc",
		StoreDigest: digest, BaselineDigest: digest, CreatedAt: now, UpdatedAt: now,
	}, state.Binding{
		SourceID: src.ID, RelativeDir: relDir, Digest: digest,
		SourceCommit: src.LastCommit, ImportedAt: now,
	}, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	op.SkillID = id
	return op
}

// openOperations returns the current unfinished operation intents.
func openOperations(t *testing.T, a *App) []skillstore.Operation {
	t.Helper()
	ops, err := state.ListOpenOperations(a.db)
	if err != nil {
		t.Fatal(err)
	}
	return ops
}
