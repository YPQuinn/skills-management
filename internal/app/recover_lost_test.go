package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

func writeLiveSkill(t *testing.T, store, slug, name, extra string) {
	t.Helper()
	dir := filepath.Join(store, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: desc\n---\n# " + name + "\n" + extra
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = "symlink:" + target
			return nil
		}
		if d.IsDir() {
			out[filepath.ToSlash(rel)] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRecoverLostStoreAdoptsValidSkillsAndPreservesInternal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLiveSkill(t, store, "alpha", "Alpha", "keep-me\n")
	writeLiveSkill(t, store, "beta", "Beta", "")
	if err := os.WriteFile(filepath.Join(store, "notes.txt"), []byte("leave"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store, "Not-A-Slug"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store, "empty-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(store, ".skillctl", "baselines", "7")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leftover, "SKILL.md"), []byte("old baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store, ".skillctl", "previous", "7"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, ".skillctl", "previous", "7", "old.md"), []byte("snap\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store, ".skillctl", "staging", "3"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := hashTree(t, store)

	a, err := New(store, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	result, err := a.RecoverLostStore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	after := hashTree(t, store)
	if len(before) != len(after) {
		t.Fatalf("store tree size changed: before %d after %d", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("store path %s rewritten: %q -> %q", p, h, after[p])
		}
	}
	if len(result.Recovered) != 2 || result.Recovered[0].Slug != "alpha" || result.Recovered[1].Slug != "beta" {
		t.Fatalf("recovered: %+v", result.Recovered)
	}
	if result.Recovered[0].ID <= 7 {
		t.Fatalf("recovered Skill id collided with leftover Baseline id: %+v", result.Recovered[0])
	}
	if result.Recovered[0].Name != "Alpha" || result.Recovered[0].StoreDigest == "" {
		t.Fatalf("recovered identity: %+v", result.Recovered[0])
	}
	wantPreserved := []string{".skillctl/baselines/7", ".skillctl/previous/7", ".skillctl/staging/3"}
	if len(result.PreservedInternal) != len(wantPreserved) {
		t.Fatalf("preserved: %v", result.PreservedInternal)
	}
	for i, p := range wantPreserved {
		if result.PreservedInternal[i] != p {
			t.Fatalf("preserved[%d]=%q want %q", i, result.PreservedInternal[i], p)
		}
	}
	if len(result.Unrecoverable) != 5 {
		t.Fatalf("unrecoverable: %v", result.Unrecoverable)
	}
	skipped := map[string]bool{}
	for _, s := range result.Skipped {
		skipped[s.Name] = true
	}
	if !skipped["notes.txt"] || !skipped["Not-A-Slug"] || !skipped["empty-dir"] {
		t.Fatalf("skipped: %+v", result.Skipped)
	}

	skills, err := a.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 || skills[0].Binding != nil || skills[0].SyncStatus != "unbound" {
		t.Fatalf("listed skills: %+v", skills)
	}
	if skills[0].HasPreviousSnapshot || skills[0].BaselineDigest != "" {
		t.Fatalf("must not invent Baseline/snapshot: %+v", skills[0])
	}
	wantDigest, err := source.TreeDigest(context.Background(), filepath.Join(store, "alpha"))
	if err != nil || skills[0].StoreDigest != wantDigest {
		t.Fatalf("store digest: %q vs %q (%v)", skills[0].StoreDigest, wantDigest, err)
	}
	sources, err := a.ListSources()
	if err != nil || len(sources) != 0 {
		t.Fatalf("sources must be empty: %+v, %v", sources, err)
	}
	groups, err := a.ListGroups()
	if err != nil || len(groups) != 0 {
		t.Fatalf("groups must be empty: %+v, %v", groups, err)
	}
	targets, err := a.ListTargets()
	if err != nil || len(targets) != 0 {
		t.Fatalf("targets must be empty: %+v, %v", targets, err)
	}

	// A later import must not reuse leftover staging/operation ids.
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		Slug: "gamma", Kind: skillstore.KindImport, NewDigest: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if op.ID <= 3 {
		t.Fatalf("operation id reused leftover staging id: %d", op.ID)
	}
}

func TestRecoverLostStoreRefusesSymlinkSwapOfListedName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLiveSkill(t, store, "alpha", "Alpha", "original-bytes\n")
	before := hashTree(t, store)

	evil := filepath.Join(dir, "evil")
	writeLiveSkill(t, evil, "alpha", "Evil", "attacker\n")
	parked := filepath.Join(dir, "parked-alpha")

	a, err := New(store, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	a.recoverNameHook = func(name string) {
		if name != "alpha" {
			return
		}
		if err := os.Rename(filepath.Join(store, "alpha"), parked); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(evil, "alpha"), filepath.Join(store, "alpha")); err != nil {
			t.Fatal(err)
		}
	}
	result, err := a.RecoverLostStore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range result.Recovered {
		if rec.Slug == "alpha" {
			t.Fatalf("swapped symlink must not be adopted: %+v", rec)
		}
		if strings.Contains(rec.Name, "Evil") {
			t.Fatalf("foreign content adopted: %+v", rec)
		}
	}
	skipped := false
	for _, s := range result.Skipped {
		if s.Name == "alpha" {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("swapped alpha must be skipped: %+v", result.Skipped)
	}
	if data, err := os.ReadFile(filepath.Join(parked, "SKILL.md")); err != nil || !strings.Contains(string(data), "original-bytes") {
		t.Fatalf("original tree must be preserved: %q %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(evil, "alpha", "SKILL.md")); err != nil || !strings.Contains(string(data), "attacker") {
		t.Fatalf("foreign tree rewritten: %q %v", data, err)
	}
	// The live Store name is now a symlink; recover must not have followed
	// it and rewritten either side.
	if info, err := os.Lstat(filepath.Join(store, "alpha")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("swap symlink missing: %v", err)
	}
	_ = before
}
