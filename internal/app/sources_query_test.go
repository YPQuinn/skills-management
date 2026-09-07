package app

import (
	"context"
	"errors"
	"testing"

	"skillctl/internal/source"
)

func TestResolveSourceArg(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	if id, err := a.ResolveSourceArg("shared"); err != nil || id != src.ID {
		t.Fatalf("by name: %d, %v", id, err)
	}
	if id, err := a.ResolveSourceArg("1"); err != nil || id != 1 {
		t.Fatalf("numeric arg is the id: %d, %v", id, err)
	}
	if _, err := a.ResolveSourceArg("missing"); !isCode(err, CodeNotFound) {
		t.Fatalf("missing: %v", err)
	}

	// a second Source cannot reuse the name: the add conflicts, so name
	// resolution stays unambiguous
	other := t.TempDir()
	writeSourceSkill(t, other, "beta")
	if _, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: other, Name: "shared"}); !isCode(err, CodeConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	if id, err := a.ResolveSourceArg("shared"); err != nil || id != src.ID {
		t.Fatalf("name must still resolve to the single Source: %d, %v", id, err)
	}
}

func isCode(err error, code string) bool {
	var ae *Error
	return errors.As(err, &ae) && ae.Code == code
}
