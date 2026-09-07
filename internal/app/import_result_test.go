package app

import (
	"encoding/json"
	"testing"
	"time"
)

func TestImportStatusValues(t *testing.T) {
	t.Parallel()
	for status, want := range map[ImportStatus]string{
		StatusImported:        "imported",
		StatusAlreadyImported: "already_imported",
		StatusSkippedConflict: "skipped_conflict",
		StatusReplaced:        "replaced",
		StatusFailed:          "failed",
	} {
		if string(status) != want {
			t.Fatalf("status %q must serialize to %q", status, want)
		}
	}
}

// TestImportItemResultJSON pins the machine-consumed JSON shape of one item:
// snake_case field names, optional fields omitted when empty, and the
// replacement preview carried as the existing app Skill with its Binding.
func TestImportItemResultJSON(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	preview := &Skill{
		ID: 7, Slug: "alpha", Name: "Alpha", Description: "one",
		StoreDigest: "old-store", BaselineDigest: "old-store",
		CreatedAt: now, UpdatedAt: now,
		Binding: &SourceBinding{
			SourceID: 3, SourceName: "vercel", RelativeDir: "skills/alpha",
			Digest: "old-store", SourceCommit: "abc123", ImportedAt: now,
		},
	}
	item := ImportItemResult{
		Status:        StatusReplaced,
		RelativeDir:   "skills/alpha",
		RequestedSlug: "alpha",
		Slug:          "alpha",
		SkillID:       7,
		Replaces:      preview,
	}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["status"] != "replaced" || m["relative_dir"] != "skills/alpha" ||
		m["requested_slug"] != "alpha" || m["slug"] != "alpha" ||
		m["skill_id"] != float64(7) {
		t.Fatalf("item fields: %v", m)
	}
	if _, present := m["error_code"]; present {
		t.Fatalf("empty error_code must be omitted: %v", m)
	}
	replaces, ok := m["replaces"].(map[string]any)
	if !ok {
		t.Fatalf("replaces preview missing: %v", m)
	}
	if replaces["slug"] != "alpha" || replaces["id"] != float64(7) {
		t.Fatalf("replaces skill fields: %v", replaces)
	}
	binding, ok := replaces["binding"].(map[string]any)
	if !ok {
		t.Fatalf("replaces binding missing: %v", replaces)
	}
	if binding["source_id"] != float64(3) || binding["source_name"] != "vercel" ||
		binding["relative_dir"] != "skills/alpha" {
		t.Fatalf("replaces binding fields: %v", binding)
	}
}

func TestImportItemResultFailedFields(t *testing.T) {
	t.Parallel()
	item := ImportItemResult{
		Status:        StatusFailed,
		RelativeDir:   "skills/broken",
		RequestedSlug: "broken",
		ErrorCode:     CodeSourceUnavailable,
		ErrorMessage:  "Source is not reachable",
	}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["status"] != "failed" || m["error_code"] != CodeSourceUnavailable ||
		m["error_message"] != "Source is not reachable" {
		t.Fatalf("failed item fields: %v", m)
	}
	// no committed Skill id, so skill_id must be omitted
	if _, present := m["skill_id"]; present {
		t.Fatalf("skill_id must be omitted when absent: %v", m)
	}
	if _, present := m["slug"]; present {
		t.Fatalf("slug must be omitted when absent: %v", m)
	}
	if _, present := m["replaces"]; present {
		t.Fatalf("replaces must be omitted when absent: %v", m)
	}
}

func TestSummarizeImportItems(t *testing.T) {
	t.Parallel()
	items := []ImportItemResult{
		{Status: StatusImported, RelativeDir: "skills/a"},
		{Status: StatusImported, RelativeDir: "skills/b"},
		{Status: StatusAlreadyImported, RelativeDir: "skills/c"},
		{Status: StatusSkippedConflict, RelativeDir: "skills/d"},
		{Status: StatusReplaced, RelativeDir: "skills/e"},
		{Status: StatusFailed, RelativeDir: "skills/f"},
		{Status: "unknown", RelativeDir: "skills/g"},
	}
	summary := SummarizeImportItems(items)
	if summary.Total != 7 || summary.Imported != 2 || summary.AlreadyImported != 1 ||
		summary.SkippedConflict != 1 || summary.Replaced != 1 || summary.Failed != 1 {
		t.Fatalf("summary: %+v", summary)
	}
	if summary.Imported+summary.AlreadyImported+summary.SkippedConflict+summary.Replaced+summary.Failed != 6 {
		t.Fatalf("unknown status must not count toward a per-status bucket: %+v", summary)
	}
	if got := SummarizeImportItems(nil); got != (ImportSkillsSummary{}) {
		t.Fatalf("empty batch summary: %+v", got)
	}
}

func TestImportSkillsResultJSON(t *testing.T) {
	t.Parallel()
	result := ImportSkillsResult{
		Items: []ImportItemResult{{Status: StatusImported, RelativeDir: "skills/a"}},
		Summary: SummarizeImportItems([]ImportItemResult{
			{Status: StatusImported, RelativeDir: "skills/a"},
		}),
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items: %v", m)
	}
	summary, ok := m["summary"].(map[string]any)
	if !ok || summary["total"] != float64(1) || summary["imported"] != float64(1) {
		t.Fatalf("summary: %v", m)
	}
}
