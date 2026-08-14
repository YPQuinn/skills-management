package skillstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreUnreadableReplace locks the crash window of an unreadable-old
// replace: after Install displaced a plain file and installed the staged
// tree, the pending restore returns the plain file to the live slot by
// identity and cleans the evidence, so a failed Accept Source never loses
// the displaced content.
func TestRestoreUnreadableReplace(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(liveDir(t, s, "alpha"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	op := Operation{ID: 1, SkillID: 7, Slug: "alpha", Kind: KindReplace, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(context.Background(), op, &staged); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatalf("restore: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(s.Root, "alpha"))
	if err != nil || string(data) != "file" {
		t.Fatalf("displaced node not restored: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(s.Root, ".skillctl", "recovery", "1")); !os.IsNotExist(err) {
		t.Fatalf("recovery slot must be gone after the restore: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(s.Root, ".skillctl", "staging", "1")); !os.IsNotExist(err) {
		t.Fatalf("operation evidence must be gone after the restore: %v", err)
	}
}
