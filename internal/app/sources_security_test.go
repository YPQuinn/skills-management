package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"skillctl/internal/source"
)

// TestAddSourceRejectsCredentialURLs guards fix 2 at the application seam:
// Git URLs embedding credentials fail registration with invalid_argument,
// the error never echoes the secret, and nothing is persisted.
func TestAddSourceRejectsCredentialURLs(t *testing.T) {
	a := newTestApp(t)
	for _, loc := range []string{
		"https://user:supersecret@github.com/o/r.git",
		"https://ghp_1234567890abcdef@github.com/o/r.git",
		"https://user:pass@gitlab.com/o/r.git",
		"ssh://user:supersecret@example.com/o/r.git",
	} {
		_, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindGit, Location: loc})
		if !isCode(err, CodeInvalidArgument) {
			t.Fatalf("%s: got %v, want invalid_argument", loc, err)
		}
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		for _, secret := range []string{"supersecret", "ghp_1234567890abcdef", "user:pass"} {
			if strings.Contains(msg, secret) {
				t.Fatalf("error must not echo credentials for %s: %v", loc, err)
			}
		}
	}
	items, err := a.ListSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("credential URLs must never be saved: %+v", items)
	}
}

// TestCheckSourceRejectsSymlinkSwapIntoStore is the reproduced regression
// for fix 6: after registration, replacing the Source root with a symlink
// into the Skill Store must be rejected without scanning the Store and
// without mutating the retained Inventory or observation metadata.
func TestCheckSourceRejectsSymlinkSwapIntoStore(t *testing.T) {
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Name: "swap"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}

	// swap the registered root for a symlink pointing into the Store
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a.StorePath, root); err != nil {
		t.Fatal(err)
	}
	_, err = a.CheckSource(context.Background(), src.ID)
	if !isCode(err, CodeInvalidArgument) {
		t.Fatalf("symlink swap into the Store: got %v, want invalid_argument", err)
	}
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("swap error must explain the identity change: %v", err)
	}

	// nothing was scanned and nothing mutated: the retained Inventory,
	// digest, and check metadata are untouched
	after, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Entries) != 1 || after.Entries[0].Name != "alpha" {
		t.Fatalf("swap must not change the Inventory: %+v", after.Entries)
	}
	if after.LastInventoryDigest != before.LastInventoryDigest {
		t.Fatalf("swap must not change the inventory digest: %q -> %q", before.LastInventoryDigest, after.LastInventoryDigest)
	}
	if !after.LastCheckedAt.Equal(*before.LastCheckedAt) || after.LastCheckResult != before.LastCheckResult {
		t.Fatalf("swap must not advance check metadata: before %+v, after %+v", before.LastCheckedAt, after.LastCheckedAt)
	}
}

// TestCheckSourceSymlinkSwapToOutsideTree also fails safely: the swapped
// root may not even point into the Store, the physical identity change is
// rejected on its own.
func TestCheckSourceSymlinkSwapToOutsideTree(t *testing.T) {
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root})
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	writeSourceSkill(t, other, "beta")
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, root); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CheckSource(context.Background(), src.ID); !isCode(err, CodeInvalidArgument) {
		t.Fatalf("symlink swap to another tree: got %v, want invalid_argument", err)
	}
}
