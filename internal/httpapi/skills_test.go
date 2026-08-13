package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addAPISource registers a Local Source through the REST boundary and
// returns its id.
func addAPISource(t *testing.T, ts *testServer, root, name string) int64 {
	t.Helper()
	body := fmt.Sprintf(`{"kind": "local", "location": %q, "name": %q}`, root, name)
	resp := ts.do(t, "POST", "/api/v1/sources", body, "application/json", "", "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create source: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var created sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &created); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

func decodeSkillList(t *testing.T, body string) []skillJSON {
	t.Helper()
	var envelope struct {
		Items []skillJSON `json:"items"`
		Total int         `json:"total"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	if envelope.Total != len(envelope.Items) {
		t.Fatalf("total %d does not match items %d", envelope.Total, len(envelope.Items))
	}
	return envelope.Items
}

// TestSkillsRESTLifecycle covers list, detail, import, re-import no-op,
// conflict, and replace through the REST boundary.
func TestSkillsRESTLifecycle(t *testing.T) {
	ts := startServer(t)
	resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	// an empty Store lists as {items: [], total: 0}
	resp = ts.do(t, "GET", "/api/v1/skills", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: got %d", resp.StatusCode)
	}
	if items := decodeSkillList(t, readBody(t, resp)); len(items) != 0 {
		t.Fatalf("initial list: %+v", items)
	}

	// one Local Source with one Skill
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	sourceID := addAPISource(t, ts, root, "local-one")

	// import one entry by relative path
	body := fmt.Sprintf(`{"source_id": %d, "selectors": [{"relative_dir": "skills/alpha"}]}`, sourceID)
	resp = ts.do(t, "POST", "/api/v1/skills/import", body, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var result importSkillsResultJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Total != 1 || result.Summary.Imported != 1 {
		t.Fatalf("import summary: %+v", result.Summary)
	}
	item := result.Items[0]
	if item.Status != "imported" || item.RelativeDir != "skills/alpha" || item.Slug != "alpha" || item.SkillID == 0 {
		t.Fatalf("import item: %+v", item)
	}

	// list returns the Skill with its Binding
	resp = ts.do(t, "GET", "/api/v1/skills", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: got %d", resp.StatusCode)
	}
	items := decodeSkillList(t, readBody(t, resp))
	if len(items) != 1 {
		t.Fatalf("list after import: %+v", items)
	}
	sk := items[0]
	if sk.ID != item.SkillID || sk.Slug != "alpha" || sk.Name != "alpha" || sk.StoreDigest == "" ||
		sk.BaselineDigest == "" || sk.CreatedAt == "" || !strings.HasSuffix(sk.CreatedAt, "Z") ||
		sk.UpdatedAt == "" {
		t.Fatalf("list item: %+v", sk)
	}
	if sk.Binding == nil || sk.Binding.SourceID != sourceID || sk.Binding.SourceName != "local-one" ||
		sk.Binding.RelativeDir != "skills/alpha" || sk.Binding.Digest == "" || sk.Binding.ImportedAt == "" {
		t.Fatalf("list item binding: %+v", sk.Binding)
	}

	// detail agrees with the list item
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/skills/%d", sk.ID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("show: got %d", resp.StatusCode)
	}
	var detail skillJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Slug != sk.Slug || detail.StoreDigest != sk.StoreDigest || detail.Binding.SourceName != "local-one" {
		t.Fatalf("detail: %+v", detail)
	}

	// the same Source entry again is an already-imported no-op
	resp = ts.do(t, "POST", "/api/v1/skills/import", body, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-import: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Items[0].Status != "already_imported" || result.Summary.AlreadyImported != 1 {
		t.Fatalf("re-import: %+v", result)
	}

	// a rival Source claims the same slug with different content: skipped
	// without replace, replaced with it
	rival := t.TempDir()
	writeAPISkill(t, rival, "alpha")
	if err := os.WriteFile(filepath.Join(rival, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: desc\n---\n# Alpha v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rivalID := addAPISource(t, ts, rival, "rival")
	rivalBody := fmt.Sprintf(`{"source_id": %d, "selectors": [{"relative_dir": "skills/alpha"}]}`, rivalID)
	resp = ts.do(t, "POST", "/api/v1/skills/import", rivalBody, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("conflict import: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Items[0].Status != "skipped_conflict" || result.Items[0].SkillID != sk.ID || result.Items[0].Replaces == nil ||
		result.Items[0].Replaces.SkillID != sk.ID || result.Items[0].Replaces.Slug != "alpha" {
		t.Fatalf("conflict item: %+v", result.Items[0])
	}
	replaceBody := fmt.Sprintf(`{"source_id": %d, "selectors": [{"relative_dir": "skills/alpha", "replace": true}]}`, rivalID)
	resp = ts.do(t, "POST", "/api/v1/skills/import", replaceBody, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replace import: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Items[0].Status != "replaced" || result.Items[0].SkillID != sk.ID {
		t.Fatalf("replace item: %+v", result.Items[0])
	}
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/skills/%d", sk.ID), "", "", "", "")
	if err := json.Unmarshal([]byte(readBody(t, resp)), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Binding == nil || detail.Binding.SourceName != "rival" {
		t.Fatalf("binding after replace: %+v", detail.Binding)
	}
}

// TestSkillsRESTBatchImport covers --all equivalent batches and mixed
// per-item outcomes: a valid batch always returns HTTP 200 with every item.
func TestSkillsRESTBatchImport(t *testing.T) {
	ts := startServer(t)
	resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	writeAPISkill(t, root, "beta")
	writeAPISkill(t, root, "gamma")
	sourceID := addAPISource(t, ts, root, "batch")

	// all: true imports every entry sorted by relative directory
	resp = ts.do(t, "POST", "/api/v1/skills/import",
		fmt.Sprintf(`{"source_id": %d, "all": true}`, sourceID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import all: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var result importSkillsResultJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Total != 3 || result.Summary.Imported != 3 || len(result.Items) != 3 {
		t.Fatalf("import all: %+v", result)
	}
	for i, want := range []string{"skills/alpha", "skills/beta", "skills/gamma"} {
		if result.Items[i].RelativeDir != want || result.Items[i].Status != "imported" {
			t.Fatalf("import all item %d: %+v", i, result.Items[i])
		}
	}

	// a mixed batch reports every per-item outcome with HTTP 200
	other := t.TempDir()
	writeAPISkill(t, other, "delta")
	writeAPISkill(t, other, "alpha")
	if err := os.WriteFile(filepath.Join(other, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: desc\n---\n# Alpha v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	otherID := addAPISource(t, ts, other, "mixed")
	body := fmt.Sprintf(`{"source_id": %d, "selectors": [{"relative_dir": "skills/delta"}, {"relative_dir": "skills/alpha", "replace": true}]}`, otherID)
	resp = ts.do(t, "POST", "/api/v1/skills/import", body, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mixed batch: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Total != 2 || result.Summary.Imported != 1 || result.Summary.Replaced != 1 {
		t.Fatalf("mixed summary: %+v", result.Summary)
	}
	if result.Items[0].Status != "imported" || result.Items[0].RelativeDir != "skills/delta" {
		t.Fatalf("mixed item 0: %+v", result.Items[0])
	}
	if result.Items[1].Status != "replaced" || result.Items[1].RelativeDir != "skills/alpha" {
		t.Fatalf("mixed item 1: %+v", result.Items[1])
	}
}
