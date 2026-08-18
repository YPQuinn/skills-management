package skillstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveSkillDrainsLiveBaselineAndPrevious(t *testing.T) {
	s := newStore(t)
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("live"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.BaselineDir(7), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.BaselineDir(7), "SKILL.md"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.PreviousDir(7), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.PreviousDir(7), "SKILL.md"), []byte("prev"), 0o644); err != nil {
		t.Fatal(err)
	}

	op := Operation{ID: 1, SkillID: 7, Slug: "alpha", Kind: KindRemove}
	if err := s.IsolateRemove(op); err != nil {
		t.Fatal(err)
	}
	if err := s.FinalizeRemove(op); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(live); !os.IsNotExist(err) {
		t.Fatalf("live: %v", err)
	}
	if _, err := os.Lstat(s.BaselineDir(7)); !os.IsNotExist(err) {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := os.Lstat(s.PreviousDir(7)); !os.IsNotExist(err) {
		t.Fatalf("previous: %v", err)
	}
	if err := s.IsolateRemove(op); err != nil {
		t.Fatalf("idempotent isolate: %v", err)
	}
	if err := s.FinalizeRemove(op); err != nil {
		t.Fatalf("idempotent finalize: %v", err)
	}
}

func TestRemoveSkillRestoresLiveWhenLaterIsolateFails(t *testing.T) {
	s := newStore(t)
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("live"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.BaselineDir(7), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.BaselineDir(7), "SKILL.md"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.PreviousDir(7), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.PreviousDir(7), "SKILL.md"), []byte("prev"), 0o644); err != nil {
		t.Fatal(err)
	}

	op := Operation{ID: 1, SkillID: 7, Slug: "alpha", Kind: KindRemove}
	s.SetHook(func(p HookPoint) {
		if p != HookAfterRemoveSkillLiveMoved {
			return
		}
		if err := os.MkdirAll(filepath.Join(s.InternalDir(), "recovery", "1", "base"), 0o755); err != nil {
			t.Fatal(err)
		}
	})
	if err := s.IsolateRemove(op); err == nil {
		t.Fatal("want failure after live was isolated")
	}
	data, err := os.ReadFile(filepath.Join(live, "SKILL.md"))
	if err != nil || string(data) != "live" {
		t.Fatalf("live must be restored: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil || string(data) != "base" {
		t.Fatalf("baseline must remain: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(s.PreviousDir(7), "SKILL.md")); err != nil || string(data) != "prev" {
		t.Fatalf("previous must remain: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "recovery", "1", "live")); !os.IsNotExist(err) {
		t.Fatalf("live recovery slot must be cleared: %v", err)
	}
}

func TestRemoveSkillRefusesUnmanagedFile(t *testing.T) {
	s := newStore(t)
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	live := liveDir(t, s, "alpha")
	if err := os.WriteFile(live, []byte("not a skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	op := Operation{ID: 1, SkillID: 1, Slug: "alpha", Kind: KindRemove}
	if err := s.IsolateRemove(op); err == nil {
		t.Fatal("want unmanaged")
	}
	if _, err := os.Lstat(live); err != nil {
		t.Fatalf("unmanaged file must remain: %v", err)
	}
}
