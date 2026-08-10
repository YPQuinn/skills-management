package source

import "testing"

func TestInventoryDigestDeterministic(t *testing.T) {
	entries := []Entry{
		{RelativeDir: "skills/a", Name: "A", Digest: "da"},
		{RelativeDir: "skills/b", Name: "B", Digest: "db"},
	}
	issues := []Issue{{RelativeDir: "skills/bad", Reason: "missing name"}}
	first := inventoryDigest(entries, issues)
	second := inventoryDigest(entries, issues)
	if first != second {
		t.Fatalf("inventory digest must be deterministic: %q vs %q", first, second)
	}
	// any content or issue change rebinds the aggregate digest
	if inventoryDigest(entries, nil) == first {
		t.Fatal("dropping an issue must change the inventory digest")
	}
	changed := append([]Entry(nil), entries...)
	changed[0].Digest = "different"
	if inventoryDigest(changed, issues) == first {
		t.Fatal("an entry digest change must change the inventory digest")
	}
}

func TestInventoryDigestOrderIndependent(t *testing.T) {
	entries := []Entry{
		{RelativeDir: "skills/a", Name: "A", Digest: "da"},
		{RelativeDir: "skills/b", Name: "B", Digest: "db"},
		{RelativeDir: "skills/c", Name: "C", Digest: "dc"},
	}
	issues := []Issue{
		{RelativeDir: "skills/bad", Reason: "missing name"},
		{RelativeDir: "skills/worse", Reason: "missing description"},
	}
	ordered := inventoryDigest(entries, issues)

	reversed := append([]Entry(nil), entries...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	revIssues := append([]Issue(nil), issues...)
	for i, j := 0, len(revIssues)-1; i < j; i, j = i+1, j-1 {
		revIssues[i], revIssues[j] = revIssues[j], revIssues[i]
	}
	if got := inventoryDigest(reversed, revIssues); got != ordered {
		t.Fatalf("digest must not depend on caller order: %q vs %q", got, ordered)
	}

	// the caller's slices are never mutated
	if entries[0].RelativeDir != "skills/a" || issues[0].RelativeDir != "skills/bad" {
		t.Fatal("inventoryDigest must hash defensive copies")
	}
}
