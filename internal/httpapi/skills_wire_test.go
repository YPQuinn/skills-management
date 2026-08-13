package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// The wire-contract tests pin the raw JSON shapes the WebUI depends on with
// literals independent of the production DTO structs, so an accidental JSON
// tag or omitempty drift cannot hide behind the same struct in tests. They
// assert key presence/omission and types only, never re-testing behavior.

// rawJSON decodes one response body into an untyped object.
func rawJSON(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("body is not a JSON object: %v\n%s", err, body)
	}
	return m
}

// requireKeys asserts every key is present; forbidKeys asserts every key is
// absent, pinning the intended snake_case presence and omission.
func requireKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Fatalf("wire shape missing key %q in %v", k, m)
		}
	}
}

func forbidKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; ok {
			t.Fatalf("wire shape must omit key %q in %v", k, m)
		}
	}
}

func wiredServer(t *testing.T) *testServer {
	t.Helper()
	ts := startServer(t)
	resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
	return ts
}

func wireImport(t *testing.T, ts *testServer, sourceID int64) map[string]any {
	t.Helper()
	body := fmt.Sprintf(`{"source_id": %d, "selectors": [{"relative_dir": "skills/alpha"}]}`, sourceID)
	resp := ts.do(t, "POST", "/api/v1/skills/import", body, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	return rawJSON(t, readBody(t, resp))
}

// TestSkillsRESTWireContractCollection pins GET /api/v1/skills: an empty
// collection is {items: [], total: 0} with items never null, and a filled
// collection item carries the full snake_case Skill key set with its
// Binding (source_commit omitted for a local Source).
func TestSkillsRESTWireContractCollection(t *testing.T) {
	ts := wiredServer(t)
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	sourceID := addAPISource(t, ts, root, "wire")

	resp := ts.do(t, "GET", "/api/v1/skills", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: got %d", resp.StatusCode)
	}
	env := rawJSON(t, readBody(t, resp))
	requireKeys(t, env, "items", "total")
	items, ok := env["items"].([]any)
	if !ok || items == nil {
		t.Fatalf("items must be a present array, got %T %v", env["items"], env["items"])
	}
	if len(items) != 0 || env["total"].(float64) != 0 {
		t.Fatalf("empty collection: %v", env)
	}

	wireImport(t, ts, sourceID)
	resp = ts.do(t, "GET", "/api/v1/skills", "", "", "", "")
	env = rawJSON(t, readBody(t, resp))
	if env["total"].(float64) != 1 {
		t.Fatalf("total after import: %v", env["total"])
	}
	item := env["items"].([]any)[0].(map[string]any)
	requireKeys(t, item, "id", "slug", "name", "description", "store_digest",
		"baseline_digest", "created_at", "updated_at", "binding")
	for _, k := range []string{"created_at", "updated_at", "store_digest", "baseline_digest"} {
		if _, ok := item[k].(string); !ok {
			t.Fatalf("%s must be a string, got %T", k, item[k])
		}
	}
	binding := item["binding"].(map[string]any)
	requireKeys(t, binding, "source_id", "source_name", "relative_dir", "digest", "imported_at")
	forbidKeys(t, binding, "source_commit")
}

// TestSkillsRESTWireContractDetail pins GET /api/v1/skills/{id}: the detail
// is one Skill object (never a collection) with the same Binding key set.
func TestSkillsRESTWireContractDetail(t *testing.T) {
	ts := wiredServer(t)
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	sourceID := addAPISource(t, ts, root, "wire")
	imported := wireImport(t, ts, sourceID)
	skillID := int64(imported["items"].([]any)[0].(map[string]any)["skill_id"].(float64))

	resp := ts.do(t, "GET", fmt.Sprintf("/api/v1/skills/%d", skillID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail: got %d", resp.StatusCode)
	}
	detail := rawJSON(t, readBody(t, resp))
	requireKeys(t, detail, "id", "slug", "name", "description", "store_digest",
		"baseline_digest", "created_at", "updated_at", "binding")
	forbidKeys(t, detail, "items", "total")
	binding := detail["binding"].(map[string]any)
	requireKeys(t, binding, "source_id", "source_name", "relative_dir", "digest", "imported_at")
	forbidKeys(t, binding, "source_commit")
	if detail["slug"] != "alpha" {
		t.Fatalf("detail slug: %v", detail["slug"])
	}
}

// TestSkillsRESTWireContractImport pins POST /api/v1/skills/import: a
// success item carries status/relative_dir/requested_slug/slug/skill_id and
// omits code/message/replaces; a conflict item adds replaces with exactly
// skill_id/slug/name; the summary always carries all six tally keys.
func TestSkillsRESTWireContractImport(t *testing.T) {
	ts := wiredServer(t)
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	sourceID := addAPISource(t, ts, root, "wire")

	// success item and full summary key set
	success := wireImport(t, ts, sourceID)
	item := success["items"].([]any)[0].(map[string]any)
	requireKeys(t, item, "status", "relative_dir", "requested_slug", "slug", "skill_id")
	forbidKeys(t, item, "code", "message", "replaces", "error_code", "error_message")
	if item["status"] != "imported" || item["relative_dir"] != "skills/alpha" {
		t.Fatalf("success item: %v", item)
	}
	summary := success["summary"].(map[string]any)
	requireKeys(t, summary, "total", "imported", "already_imported", "skipped_conflict", "replaced", "failed")
	if summary["imported"].(float64) != 1 {
		t.Fatalf("summary: %v", summary)
	}

	// a rival Source claims the same slug with different content
	rival := t.TempDir()
	writeAPISkill(t, rival, "alpha")
	if err := os.WriteFile(filepath.Join(rival, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: desc\n---\n# Alpha v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rivalID := addAPISource(t, ts, rival, "wire-rival")
	conflict := wireImport(t, ts, rivalID)
	citem := conflict["items"].([]any)[0].(map[string]any)
	requireKeys(t, citem, "status", "relative_dir", "requested_slug", "slug", "skill_id", "replaces")
	forbidKeys(t, citem, "code", "message")
	if citem["status"] != "skipped_conflict" {
		t.Fatalf("conflict item: %v", citem)
	}
	replaces := citem["replaces"].(map[string]any)
	requireKeys(t, replaces, "skill_id", "slug", "name")
	forbidKeys(t, replaces, "id", "description", "store_digest", "binding")
	if replaces["slug"] != "alpha" {
		t.Fatalf("replaces: %v", replaces)
	}
	csummary := conflict["summary"].(map[string]any)
	requireKeys(t, csummary, "total", "imported", "already_imported", "skipped_conflict", "replaced", "failed")
	if csummary["skipped_conflict"].(float64) != 1 {
		t.Fatalf("conflict summary: %v", csummary)
	}
}
