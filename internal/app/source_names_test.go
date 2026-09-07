package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/source"
)

// TestAddSourceNameValidation pins the browser-contract name rules: names
// must survive trimming and be safe as one URL path segment.
func TestAddSourceNameValidation(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")

	for _, name := range []string{"bad/name", ".", ".."} {
		if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Name: name}); !isCode(err, CodeInvalidArgument) {
			t.Errorf("name %q: got %v, want invalid_argument", name, err)
		}
	}
	// a whitespace-only explicit name falls back to the default, which is
	// valid; nothing above was saved
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Name: "   "}); err != nil {
		t.Fatalf("whitespace name must fall back to the default: %v", err)
	}
	items, err := a.ListSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != filepath.Base(root) {
		t.Fatalf("saved sources: %+v", items)
	}
}

// TestAddSourceNameConflictDistinctFromLocator proves the two unique
// constraints produce distinct, clear conflict messages.
func TestAddSourceNameConflictDistinctFromLocator(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	other := t.TempDir()
	writeSourceSkill(t, other, "beta")

	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Name: "dup"}); err != nil {
		t.Fatal(err)
	}

	// same name, different locator: the message names the Source
	_, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: other, Name: "dup"})
	if !isCode(err, CodeConflict) || !strings.Contains(err.Error(), `"dup"`) {
		t.Fatalf("duplicate name: got %v, want a conflict naming the Source", err)
	}

	// same locator, different name: the message names the tuple
	_, err = a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Name: "dup-again"})
	if !isCode(err, CodeConflict) || !strings.Contains(err.Error(), "location, ref, and subpath") {
		t.Fatalf("duplicate locator: got %v, want a conflict naming the tuple", err)
	}
}
