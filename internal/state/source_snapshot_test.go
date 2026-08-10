package state

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"skillctl/internal/source"
)

// observationGen builds one full observation generation whose aggregate
// digest and Inventory belong together.
func observationGen(digest, dir string) source.Source {
	now := time.Now().UTC()
	return source.Source{
		Locator:               source.Locator{Kind: source.KindLocal, Location: "/tmp/gen"},
		UpdatedAt:             now,
		Available:             true,
		LastCheckStartedAt:    &now,
		LastCheckedAt:         &now,
		LastCheckResult:       source.CheckResultOK,
		LastSuccessfulCheckAt: &now,
		LastInventoryDigest:   digest,
		Entries:               []source.Entry{{RelativeDir: dir, Name: dir, Description: "d", Digest: digest + "-e"}},
	}
}

// TestGetSourceObservationGenerationsNeverMix is the deterministic
// concurrency regression for the coherent read: with a single connection,
// every statement is serialized, so the previous GetSource released the
// connection between the metadata read and the inventory read and a
// flipping writer could commit between them, producing a mixed generation.
// The current GetSource holds one read transaction for all three reads, so
// it can only ever observe one full generation.
func TestGetSourceObservationGenerationsNeverMix(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	id, err := InsertSource(db, observationGen("g1", "skills/a"))
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			gen := observationGen("g2", "skills/b")
			if i%2 == 0 {
				gen = observationGen("g1", "skills/a")
			}
			if err := ReplaceSourceObservation(db, id, gen); err != nil {
				t.Errorf("writer: %v", err)
				return
			}
		}
	}()
	defer func() {
		close(stop)
		wg.Wait()
	}()

	for i := 0; i < 300; i++ {
		got, err := GetSource(db, id)
		if err != nil {
			t.Fatal(err)
		}
		switch got.LastInventoryDigest {
		case "g1":
			if len(got.Entries) != 1 || got.Entries[0].RelativeDir != "skills/a" {
				t.Fatalf("mixed generation: g1 metadata with inventory %+v", got.Entries)
			}
		case "g2":
			if len(got.Entries) != 1 || got.Entries[0].RelativeDir != "skills/b" {
				t.Fatalf("mixed generation: g2 metadata with inventory %+v", got.Entries)
			}
		default:
			t.Fatalf("unknown generation digest %q", got.LastInventoryDigest)
		}
	}
}

// TestGetSourceNeverSeesUncommittedWrites pins the snapshot property: a
// writer that has updated the row and deleted the Inventory but not yet
// committed must be invisible to a concurrent reader, which returns the
// previous committed generation in full.
func TestGetSourceNeverSeesUncommittedWrites(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := InsertSource(db, observationGen("g1", "skills/a"))
	if err != nil {
		t.Fatal(err)
	}

	wtx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wtx.Exec(`UPDATE sources SET last_inventory_digest = 'g2' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := wtx.Exec(`DELETE FROM source_inventory WHERE source_id = ?`, id); err != nil {
		t.Fatal(err)
	}

	got, err := GetSource(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastInventoryDigest != "g1" || len(got.Entries) != 1 || got.Entries[0].RelativeDir != "skills/a" {
		t.Fatalf("reader saw uncommitted writes: %+v", got)
	}
	if err := wtx.Rollback(); err != nil {
		t.Fatal(err)
	}

	// after the writer commits a full generation, the reader sees it whole
	wtx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wtx.Exec(`UPDATE sources SET last_inventory_digest = 'g2' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := wtx.Exec(`DELETE FROM source_inventory WHERE source_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := wtx.Exec(`INSERT INTO source_inventory (source_id, relative_dir, name, description, digest) VALUES (?, 'skills/b', 'b', 'd', 'g2-e')`, id); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err = GetSource(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastInventoryDigest != "g2" || len(got.Entries) != 1 || got.Entries[0].RelativeDir != "skills/b" {
		t.Fatalf("reader after commit: %+v", got)
	}
}
