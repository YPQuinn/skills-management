package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/source"
)

const testSkill = "---\nname: %s\ndescription: %s\n---\n# Body\n"

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	// initialization creates the Store; tests mirror that so overlap checks
	// can resolve the real Store path.
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := New(store, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func writeSourceSkill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte(strings.ReplaceAll(strings.ReplaceAll(testSkill, "%s", name), "%s", "desc")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeObserver returns a fixed error from Observe, letting app tests drive
// observer failure modes deterministically.
type fakeObserver struct {
	err error
}

func (f fakeObserver) Observe(context.Context, source.Locator, string) (source.Observation, error) {
	return source.Observation{}, f.err
}

// cancelObserver cancels the context during Observe and returns a valid
// observation, letting app tests prove that cancellation before persistence
// never mutates state.
type cancelObserver struct {
	cancel func()
}

func (c cancelObserver) Observe(ctx context.Context, _ source.Locator, _ string) (source.Observation, error) {
	c.cancel()
	return source.Observation{
		Digest: "digest",
		Entries: []source.Entry{
			{RelativeDir: "skills/alpha", Name: "alpha", Description: "desc", Digest: "entry-digest"},
		},
	}, nil
}

func TestAddSourceLocal(t *testing.T) {
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")

	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Name: "local-one"})
	if err != nil {
		t.Fatal(err)
	}
	if src.ID == 0 || src.Name != "local-one" || !src.Available {
		t.Fatalf("source: %+v", src)
	}
	if len(src.Entries) != 1 || src.Entries[0].Name != "alpha" {
		t.Fatalf("entries: %+v", src.Entries)
	}
	if src.LastCheckResult != source.CheckResultOK || src.LastInventoryDigest == "" {
		t.Fatalf("registration check metadata: %+v", src)
	}
	if src.Entries[0].Digest == "" {
		t.Fatalf("entry tree digest missing: %+v", src.Entries[0])
	}

	// a missing directory cannot be registered and is not saved
	missing := filepath.Join(root, "..", "other-dir")
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: missing}); !isCode(err, CodeSourceUnavailable) {
		t.Fatalf("missing directory: %v", err)
	}

	// an empty name defaults to the location base name
	other := t.TempDir()
	writeSourceSkill(t, other, "gamma")
	named, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: other})
	if err != nil {
		t.Fatal(err)
	}
	if named.Name != filepath.Base(other) {
		t.Fatalf("default name: got %q, want %q", named.Name, filepath.Base(other))
	}
}

func TestAddSourceLocalStoreOverlap(t *testing.T) {
	a := newTestApp(t)

	// the Source containing the Store is rejected
	ancestor := filepath.Dir(a.StorePath)
	writeSourceSkill(t, ancestor, "alpha")
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: ancestor}); !isCode(err, CodeInvalidArgument) {
		t.Fatalf("source containing the Store: %v", err)
	}

	// a Source inside the Store is rejected
	inside := filepath.Join(a.StorePath, "skills")
	writeSourceSkill(t, inside, "beta")
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: inside}); !isCode(err, CodeInvalidArgument) {
		t.Fatalf("source inside the Store: %v", err)
	}

	// the Store itself as a Source is rejected
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: a.StorePath}); !isCode(err, CodeInvalidArgument) {
		t.Fatalf("Store as source: %v", err)
	}

	// non-overlapping Sources still register
	sibling := t.TempDir()
	writeSourceSkill(t, sibling, "gamma")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: sibling})
	if err != nil {
		t.Fatalf("non-overlapping source: %v", err)
	}
	if src.Name != filepath.Base(sibling) {
		t.Fatalf("non-overlapping source name: %q", src.Name)
	}
	// only the non-overlapping Source was saved
	items, err := a.ListSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("rejected Sources must not be saved: %+v", items)
	}
}

func TestAddSourceCancellationSavesNothing(t *testing.T) {
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")

	ctx, cancel := context.WithCancel(context.Background())
	a.observer = cancelObserver{cancel: cancel}
	if _, err := a.AddSource(ctx, source.AddInput{Kind: source.KindLocal, Location: root}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled add: %v", err)
	}
	items, err := a.ListSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("a cancelled registration must not save a Source: %+v", items)
	}
}

func TestAddSourceValidatesAndConflicts(t *testing.T) {
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")

	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: "", Name: "x"}); !isCode(err, CodeInvalidArgument) {
		t.Fatalf("empty location: %v", err)
	}
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Ref: "main"}); !isCode(err, CodeInvalidArgument) {
		t.Fatalf("local with ref: %v", err)
	}
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Subpath: "skills"}); !isCode(err, CodeInvalidArgument) {
		t.Fatalf("local with subpath: %v", err)
	}
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindGit, Location: "not a git location"}); !isCode(err, CodeInvalidArgument) {
		t.Fatalf("bad git location: %v", err)
	}

	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root}); !isCode(err, CodeConflict) {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestAddSourceUnreachableNotSaved(t *testing.T) {
	a := newTestApp(t)
	_, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: filepath.Join(t.TempDir(), "missing")})
	if !isCode(err, CodeSourceUnavailable) {
		t.Fatalf("unreachable: %v", err)
	}
	items, err := a.ListSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("unreachable Source must not be saved: %+v", items)
	}
}
