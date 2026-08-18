package state

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"skillctl/internal/skillstore"
)

// v5FS returns a migration set containing only 001-005, the schema the
// Stage-A build left on disk.
func v5FS(t *testing.T) *fstest.MapFS {
	t.Helper()
	fsys := &fstest.MapFS{}
	for _, name := range []string{
		"001_initial.sql", "002_sources.sql", "003_source_observation.sql",
		"004_unique_source_name.sql", "005_skills.sql",
	} {
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		(*fsys)["migrations/"+name] = &fstest.MapFile{Data: data}
	}
	return fsys
}

// TestMigration006TerminalReceipts is the upgrade path: a database created
// by the Stage-A build (schema version 5) holds pending and committed
// operation rows without receipts. Opening it with the current executable
// applies migration 006, which rebuilds store_operations with the terminal
// phases and the nullable receipt column while preserving every row and id.
func TestMigration006TerminalReceipts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, v5FS(t)); err != nil {
		t.Fatal(err)
	}
	if got := appliedVersion(t, db); got != 5 {
		t.Fatalf("legacy schema: got %d, want 5", got)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO skills
		(id, slug, name, description, store_digest, baseline_digest, created_at, updated_at)
		VALUES (7, 'beta', 'Beta', '', 'old', 'old', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO store_operations
		(skill_id, slug, kind, old_digest, new_digest, phase, created_at, updated_at)
		VALUES (NULL, 'alpha', 'import', '', 'd1', 'pending', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO store_operations
		(skill_id, slug, kind, old_digest, new_digest, phase, created_at, updated_at)
		VALUES (7, 'beta', 'replace', 'old', 'd2', 'committed', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := appliedVersion(t, db); got != 11 {
		t.Fatalf("schema after upgrade: got %d, want 11", got)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("operation rows after upgrade: %d, want 2", len(ops))
	}
	if ops[0].Slug != "alpha" || ops[0].Phase != "pending" || ops[0].SkillID != 0 || ops[0].Receipt != nil {
		t.Fatalf("pending row after upgrade: %+v", ops[0])
	}
	if ops[1].Slug != "beta" || ops[1].Phase != "committed" || ops[1].SkillID != 7 || ops[1].Receipt != nil {
		t.Fatalf("committed row after upgrade: %+v", ops[1])
	}
}

// TestMigration006PreservesAUTOINCREMENTSequence proves the rebuild keeps
// the sqlite_sequence high-water mark: operation ids name filesystem
// staging paths, so reusing an id after a rebuild from an empty row set
// could collide with a leftover operation directory. A v5 database that
// allocated a high id and then deleted every row (deleting rows never
// resets sqlite_sequence) must still issue a strictly greater id after the
// upgrade.
func TestMigration006PreservesAUTOINCREMENTSequence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, v5FS(t)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`INSERT INTO store_operations
			(slug, kind, new_digest, phase, created_at, updated_at)
			VALUES ('alpha', 'import', 'd', 'pending', ?, ?)`, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`DELETE FROM store_operations`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	op, err := InsertOperation(db, skillstore.Operation{
		Slug: "beta", Kind: skillstore.KindImport, NewDigest: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if op.ID <= 3 {
		t.Fatalf("operation id %d was reused after the rebuild; the sequence high-water mark is lost", op.ID)
	}
}

// TestMigration006ReceiptCheckConstraint pins the rebuild's phase/receipt
// invariant: pending/committed/aborted rows require a NULL receipt and
// finalized/restored rows require a non-empty BLOB receipt, so a receipt
// can never be detached from a terminal row, attached to a non-terminal
// one, or stored in a non-BLOB storage class that bypasses the byte-exact
// CAS deletion.
func TestMigration006ReceiptCheckConstraint(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	base := func(phase string, receipt any) string {
		return `INSERT INTO store_operations
			(skill_id, slug, kind, old_digest, new_digest, phase, terminal_receipt, created_at, updated_at)
			VALUES (NULL, 'alpha', 'import', '', 'd1', '` + phase + `', ?, ?, ?)`
	}
	tests := []struct {
		name    string
		phase   string
		receipt any
		wantErr string
	}{
		{name: "pending with receipt", phase: "pending", receipt: []byte("r"), wantErr: "CHECK"},
		{name: "committed with receipt", phase: "committed", receipt: []byte("r"), wantErr: "CHECK"},
		{name: "aborted with receipt", phase: "aborted", receipt: []byte("r"), wantErr: "CHECK"},
		{name: "finalized without receipt", phase: "finalized", receipt: nil, wantErr: "CHECK"},
		{name: "finalized with empty receipt", phase: "finalized", receipt: []byte{}, wantErr: "CHECK"},
		{name: "restored without receipt", phase: "restored", receipt: nil, wantErr: "CHECK"},
		{name: "finalized with receipt", phase: "finalized", receipt: []byte("r"), wantErr: ""},
		{name: "restored with receipt", phase: "restored", receipt: []byte("r"), wantErr: ""},
		{name: "finalized with TEXT receipt", phase: "finalized", receipt: "text-receipt", wantErr: "CHECK"},
		{name: "restored with INTEGER receipt", phase: "restored", receipt: int64(42), wantErr: "CHECK"},
		{name: "finalized with REAL receipt", phase: "finalized", receipt: float64(1.5), wantErr: "CHECK"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.Exec(base(tt.phase, tt.receipt), tt.receipt, now, now); err != nil {
				if tt.wantErr == "" {
					t.Fatalf("valid terminal row: %v", err)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("row refusal: got %v, want %q", err, tt.wantErr)
				}
			} else if tt.wantErr != "" {
				t.Fatalf("row %q: want %q, got success", tt.name, tt.wantErr)
			}
		})
	}
}
