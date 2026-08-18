package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestInsertUnboundSkillAndReserveIDs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := ReserveAutoIncrement(db, "skills", 40); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	id, err := InsertUnboundSkill(db, Skill{
		Slug: "alpha", Name: "Alpha", Description: "desc",
		StoreDigest: "abc", BaselineDigest: "", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id <= 40 {
		t.Fatalf("id %d did not honor reserved sequence", id)
	}
	s, b, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if b != nil {
		t.Fatalf("unbound skill must have no Binding: %+v", b)
	}
	if s.SyncStatus != "unbound" || s.StoreDigest != "abc" || s.BaselineDigest != "" {
		t.Fatalf("skill: %+v", s)
	}
}
