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

// addAPIAndImport registers a Local Source and imports one entry through
// the REST boundary, returning the Source id and the Skill id.
func addAPIAndImport(t *testing.T, ts *testServer, skill string) (int64, int64) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "skills", skill)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: "+skill+"\ndescription: desc\n---\n# Body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcID := addAPISource(t, ts, root, "src-"+skill)
	resp := ts.do(t, "POST", "/api/v1/skills/import",
		fmt.Sprintf(`{"source_id": %d, "selectors": [{"relative_dir": %q}]}`, srcID, "skills/"+skill),
		"application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var result importSkillsResultJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].SkillID == 0 {
		t.Fatalf("import items: %+v", result.Items)
	}
	return srcID, result.Items[0].SkillID
}

func postEmptyJSON(t *testing.T, ts *testServer, path string) *http.Response {
	t.Helper()
	return ts.do(t, "POST", path, "{}", "application/json", "", "")
}

// decodeSyncItem decodes one single-skill sync outcome JSON value.
func decodeSyncItem(t *testing.T, body string) syncItemResultJSON {
	t.Helper()
	var out syncItemResultJSON
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return out
}

// TestSyncRESTLifecycle locks the synchronization REST contract: check,
// sync, keep-store, accept-source, rollback, diff, and the batch source
// sync, with their status codes and JSON shapes.
func TestSyncRESTLifecycle(t *testing.T) {
	ts := startServer(t)
	if resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: %d %s", resp.StatusCode, readBody(t, resp))
	}
	srcID, skillID := addAPIAndImport(t, ts, "demo")

	// check: the Skill resource carries the evaluated in_sync state.
	resp := postEmptyJSON(t, ts, fmt.Sprintf("/api/v1/skills/%d/check", skillID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var checked skillJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != "in_sync" || checked.SyncStale {
		t.Fatalf("checked skill: %+v", checked)
	}
	if checked.HasPreviousSnapshot {
		t.Fatalf("a fresh import must report no previous snapshot: %+v", checked)
	}

	// sync: in_sync is a no-op.
	resp = postEmptyJSON(t, ts, fmt.Sprintf("/api/v1/skills/%d/sync", skillID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync: %d %s", resp.StatusCode, readBody(t, resp))
	}
	if item := decodeSyncItem(t, readBody(t, resp)); item.Result != "no_op" || item.SkillID != skillID {
		t.Fatalf("sync item: %+v", item)
	}

	// Modify the Source content; the batch source sync updates it.
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/sources/%d", srcID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("show source: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var src sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &src); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(src.Location, "skills", "demo", "notes.md")
	if err := os.WriteFile(notes, []byte("upstream\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// diff shows the baseline→source addition with unified text.
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/skills/%d/diff", skillID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("diff: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var diff diffJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &diff); err != nil {
		t.Fatal(err)
	}
	if len(diff.Comparisons) != 3 || diff.Comparisons[0].From != "baseline" || diff.Comparisons[0].To != "source" {
		t.Fatalf("diff shape: %+v", diff)
	}
	found := false
	for _, e := range diff.Comparisons[0].Entries {
		if e.Path == "notes.md" && e.Text != nil && strings.Contains(e.Text.Unified, "+upstream") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diff text missing: %+v", diff.Comparisons[0].Entries)
	}

	// batch sync updates the Skill.
	resp = postEmptyJSON(t, ts, fmt.Sprintf("/api/v1/sources/%d/sync", srcID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("source sync: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var batch syncBatchResultJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &batch); err != nil {
		t.Fatal(err)
	}
	if batch.Summary.Total != 1 || batch.Summary.Updated != 1 || batch.Items[0].Result != "updated" {
		t.Fatalf("batch: %+v", batch)
	}

	// The synchronization update rotated a previous snapshot, so the Skill
	// resource now advertises has_previous_snapshot for the Rollback gate.
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/skills/%d", skillID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("show skill: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var updated skillJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &updated); err != nil {
		t.Fatal(err)
	}
	if !updated.HasPreviousSnapshot {
		t.Fatalf("a rotated snapshot must advertise has_previous_snapshot: %+v", updated)
	}

	// rollback restores the previous snapshot.
	resp = postEmptyJSON(t, ts, fmt.Sprintf("/api/v1/skills/%d/rollback", skillID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rollback: %d %s", resp.StatusCode, readBody(t, resp))
	}
	if item := decodeSyncItem(t, readBody(t, resp)); item.Result != "rolled_back" || item.Status != "store_changed" {
		t.Fatalf("rollback item: %+v", item)
	}
	// The reverse rollback restores the accepted content.
	resp = postEmptyJSON(t, ts, fmt.Sprintf("/api/v1/skills/%d/rollback", skillID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reverse rollback: %d %s", resp.StatusCode, readBody(t, resp))
	}
	if item := decodeSyncItem(t, readBody(t, resp)); item.Result != "rolled_back" {
		t.Fatalf("reverse rollback item: %+v", item)
	}

	// keep-store over the already-in-sync Skill is a no-op.
	resp = postEmptyJSON(t, ts, fmt.Sprintf("/api/v1/skills/%d/keep-store", skillID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("keep-store: %d %s", resp.StatusCode, readBody(t, resp))
	}
	if item := decodeSyncItem(t, readBody(t, resp)); item.Result != "no_op" {
		t.Fatalf("keep-store item: %+v", item)
	}

	// accept-source over an in-sync Skill is a no-op.
	resp = postEmptyJSON(t, ts, fmt.Sprintf("/api/v1/skills/%d/accept-source", skillID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("accept-source: %d %s", resp.StatusCode, readBody(t, resp))
	}
	if item := decodeSyncItem(t, readBody(t, resp)); item.Result != "no_op" {
		t.Fatalf("accept-source item: %+v", item)
	}
}

// TestSyncMutationBodyContract locks the strict empty-object decoding of
// the parameter-less mutation routes (check, sync, keep-store,
// accept-source, rollback, and the Source batch sync): an invalid body, an
// array, null, a non-empty object, or trailing data is rejected with 400
// before the mutation runs, while exactly one empty object still works.
func TestSyncMutationBodyContract(t *testing.T) {
	ts := startServer(t)
	if resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: %d %s", resp.StatusCode, readBody(t, resp))
	}
	srcID, skillID := addAPIAndImport(t, ts, "demo")
	routes := []string{
		fmt.Sprintf("/api/v1/skills/%d/check", skillID),
		fmt.Sprintf("/api/v1/skills/%d/sync", skillID),
		fmt.Sprintf("/api/v1/skills/%d/keep-store", skillID),
		fmt.Sprintf("/api/v1/skills/%d/accept-source", skillID),
		fmt.Sprintf("/api/v1/skills/%d/rollback", skillID),
		fmt.Sprintf("/api/v1/sources/%d/sync", srcID),
	}
	for _, route := range routes {
		for _, body := range []string{`null`, `[]`, `"x"`, `{"extra":1}`, `{} trailing`, ``} {
			resp := ts.do(t, "POST", route, body, "application/json", "", "")
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s body %q: %d %s", route, body, resp.StatusCode, readBody(t, resp))
			}
			if got := readBody(t, resp); !strings.Contains(got, `"code":"bad_request"`) {
				t.Fatalf("%s body %q envelope: %s", route, body, got)
			}
		}
		// Exactly one empty object still works (rollback reports conflict
		// without a snapshot, which proves the route ran).
		resp := postEmptyJSON(t, ts, route)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
			t.Fatalf("%s empty object: %d %s", route, resp.StatusCode, readBody(t, resp))
		}
	}
}

// TestSyncMutationBodyNeverExecutes locks the execution boundary: an
// invalid body is rejected before the mutation runs, so the Skill's latest
// synchronization outcome stays exactly as it was.
func TestSyncMutationBodyNeverExecutes(t *testing.T) {
	ts := startServer(t)
	if resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: %d %s", resp.StatusCode, readBody(t, resp))
	}
	_, skillID := addAPIAndImport(t, ts, "demo")
	show := func() string {
		resp := ts.do(t, "GET", fmt.Sprintf("/api/v1/skills/%d", skillID), "", "", "", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("show skill: %d %s", resp.StatusCode, readBody(t, resp))
		}
		return readBody(t, resp)
	}
	before := show()
	for _, route := range []string{
		fmt.Sprintf("/api/v1/skills/%d/check", skillID),
		fmt.Sprintf("/api/v1/skills/%d/sync", skillID),
		fmt.Sprintf("/api/v1/skills/%d/keep-store", skillID),
		fmt.Sprintf("/api/v1/skills/%d/accept-source", skillID),
		fmt.Sprintf("/api/v1/skills/%d/rollback", skillID),
	} {
		resp := ts.do(t, "POST", route, `{"extra":1}`, "application/json", "", "")
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", route, resp.StatusCode, readBody(t, resp))
		}
		if after := show(); after != before {
			t.Fatalf("an invalid %s body must not execute: %s -> %s", route, before, after)
		}
	}
}

// TestSyncRESTErrors locks the error mapping: unknown Skill, unbound
// request shape, missing mutations Content-Type enforcement, and the
// no-snapshot rollback conflict.
func TestSyncRESTErrors(t *testing.T) {
	ts := startServer(t)
	if resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: %d %s", resp.StatusCode, readBody(t, resp))
	}
	_, skillID := addAPIAndImport(t, ts, "demo")

	// Unknown Skill id: 404 not_found.
	resp := postEmptyJSON(t, ts, "/api/v1/skills/999999/sync")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown skill: %d %s", resp.StatusCode, readBody(t, resp))
	}
	// Non-numeric id: 404.
	resp = ts.do(t, "POST", "/api/v1/skills/nope/sync", "{}", "application/json", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-numeric id: %d %s", resp.StatusCode, readBody(t, resp))
	}
	// Mutation without Content-Type: 415.
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/skills/%d/sync", skillID), "{}", "", "", "")
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("content-type: %d %s", resp.StatusCode, readBody(t, resp))
	}
	// Rollback without a snapshot: 409 conflict.
	resp = postEmptyJSON(t, ts, fmt.Sprintf("/api/v1/skills/%d/rollback", skillID))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("rollback without snapshot: %d %s", resp.StatusCode, readBody(t, resp))
	}
	if body := readBody(t, resp); !strings.Contains(body, `"code":"conflict"`) {
		t.Fatalf("rollback error envelope: %s", body)
	}
	// GET diff on a mutation-only route shape is fine; an unknown API path
	// stays a JSON 404.
	resp = ts.do(t, "GET", "/api/v1/skills/1/diff/nope", "", "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown api path: %d %s", resp.StatusCode, readBody(t, resp))
	}
}
