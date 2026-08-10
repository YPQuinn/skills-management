package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTimestampFractionalPrecisionRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := testSource("alpha")
	// two observations within the same second, distinguishable only by
	// their fractional part
	base := time.Now().UTC().Truncate(time.Second)
	checked := base.Add(250 * time.Millisecond)
	s.LastCheckStartedAt = &base
	s.LastCheckedAt = &checked
	id, err := InsertSource(db, s)
	if err != nil {
		t.Fatal(err)
	}

	got, err := GetSource(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastCheckStartedAt == nil || got.LastCheckedAt == nil {
		t.Fatal("times missing")
	}
	if !got.LastCheckStartedAt.Equal(base) {
		t.Fatalf("started: got %v, want %v", got.LastCheckStartedAt, base)
	}
	if !got.LastCheckedAt.Equal(checked) {
		t.Fatalf("checked: got %v, want %v", got.LastCheckedAt, checked)
	}
	if got.LastCheckedAt.Equal(*got.LastCheckStartedAt) {
		t.Fatal("fractional precision lost: the two times must stay distinct")
	}
}

func TestTimestampParsesLegacySecondPrecision(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := InsertSource(db, testSource("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	// rewrite a stored timestamp to the legacy second-precision form that
	// earlier builds wrote; it must still parse
	if _, err := db.Exec(`UPDATE sources SET last_checked_at = '2000-01-01T00:00:00Z' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}

	got, err := GetSource(db, id)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if got.LastCheckedAt == nil || !got.LastCheckedAt.Equal(want) {
		t.Fatalf("legacy timestamp: %+v, want %v", got.LastCheckedAt, want)
	}
}
