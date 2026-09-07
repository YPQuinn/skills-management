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

const apiSkill = "---\nname: %s\ndescription: %s\n---\n# Body\n"

func readyServer(t *testing.T, setup func(root string)) *testServer {
	t.Helper()
	ts := startServer(t)
	resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
	if setup != nil {
		setup(t.TempDir())
	}
	return ts
}

func writeAPISkill(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte(fmt.Sprintf(apiSkill, name, "description of "+name)), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func decodeListSources(t *testing.T, body string) []sourceSummaryJSON {
	t.Helper()
	var envelope struct {
		Items []sourceSummaryJSON `json:"items"`
		Total int                 `json:"total"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	if envelope.Total != len(envelope.Items) {
		t.Fatalf("total %d does not match items %d", envelope.Total, len(envelope.Items))
	}
	return envelope.Items
}

func TestSourcesRESTLifecycle(t *testing.T) {
	ts := startServer(t)
	resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	// empty list
	resp = ts.do(t, "GET", "/api/v1/sources", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: got %d", resp.StatusCode)
	}
	if items := decodeListSources(t, readBody(t, resp)); len(items) != 0 {
		t.Fatalf("initial list: %+v", items)
	}

	// register a Local Source at the fixture root
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	body := fmt.Sprintf(`{"kind": "local", "location": %q, "name": "local-one"}`, root)
	resp = ts.do(t, "POST", "/api/v1/sources", body, "application/json", "", "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var created sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || !created.Available || created.Kind != "local" || created.EntryCount != 1 {
		t.Fatalf("created: %+v", created)
	}
	if len(created.Inventory) != 1 || created.Inventory[0].Name != "alpha" {
		t.Fatalf("inventory: %+v", created.Inventory)
	}
	if created.LastCheckResult != "ok" || created.LastInventoryDigest == "" {
		t.Fatalf("created check metadata: %+v", created)
	}
	if created.Inventory[0].Digest == "" {
		t.Fatalf("created entry digest missing: %+v", created.Inventory[0])
	}
	if created.CreatedAt == "" || !strings.HasSuffix(created.CreatedAt, "Z") {
		t.Fatalf("created_at must be UTC RFC3339: %q", created.CreatedAt)
	}

	// duplicate registration conflicts
	resp = ts.do(t, "POST", "/api/v1/sources", body, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "conflict" {
		t.Fatalf("duplicate code: got %q", env.Error.Code)
	}

	// list now has one item; show returns it
	resp = ts.do(t, "GET", "/api/v1/sources", "", "", "", "")
	items := decodeListSources(t, readBody(t, resp))
	if len(items) != 1 || items[0].Name != "local-one" || items[0].Stale {
		t.Fatalf("list: %+v", items)
	}
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/sources/%d", created.ID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("show: got %d", resp.StatusCode)
	}
	var shown sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.Name != "local-one" || len(shown.Inventory) != 1 {
		t.Fatalf("shown: %+v", shown)
	}

	// check records a second Skill
	writeAPISkill(t, root, "beta")
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/sources/%d/check", created.ID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var checked sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.EntryCount != 2 {
		t.Fatalf("checked inventory: %+v", checked.Inventory)
	}

	// unavailable Source: check still returns 200 with availability recorded
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/sources/%d/check", created.ID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check unavailable: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.Available || !checked.Stale || checked.LastError == "" {
		t.Fatalf("unavailable check: %+v", checked)
	}
	if len(checked.Inventory) != 2 {
		t.Fatalf("stale inventory must be retained: %+v", checked.Inventory)
	}

	// registration of an unreachable Source is an error and saves nothing
	resp = ts.do(t, "POST", "/api/v1/sources",
		fmt.Sprintf(`{"kind": "local", "location": %q}`, filepath.Join(t.TempDir(), "missing")),
		"application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("unreachable create: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "source_unavailable" {
		t.Fatalf("unreachable code: got %q", env.Error.Code)
	}
}

func TestSourcesRESTValidation(t *testing.T) {
	ts := readyServer(t, nil)
	cases := []struct {
		name, body, contentType string
		wantStatus              int
		wantCode                string
	}{
		{"invalid json", `{`, "application/json", http.StatusBadRequest, "bad_request"},
		{"unknown field", `{"location": "/x", "bogus": 1}`, "application/json", http.StatusBadRequest, "bad_request"},
		{"missing location", `{}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"bad kind", `{"kind": "svn", "location": "/tmp/x"}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"local with ref", `{"kind": "local", "location": "/tmp/x", "ref": "main"}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"local with subpath", `{"kind": "local", "location": "/tmp/x", "subpath": "skills"}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"bad git location", `{"kind": "git", "location": "plain path"}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"traversal subpath", `{"kind": "git", "location": "https://github.com/o/r.git", "subpath": "../x"}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"credential url", `{"kind": "git", "location": "https://user:supersecret@github.com/o/r.git"}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"trailing data", `{} {}`, "application/json", http.StatusBadRequest, "bad_request"},
		{"no content type", `{}`, "", http.StatusUnsupportedMediaType, "unsupported_media_type"},
	}
	for _, c := range cases {
		resp := ts.do(t, "POST", "/api/v1/sources", c.body, c.contentType, "", "")
		if resp.StatusCode != c.wantStatus {
			t.Errorf("%s: got %d, want %d (body %s)", c.name, resp.StatusCode, c.wantStatus, readBody(t, resp))
			continue
		}
		if env := decodeError(t, readBody(t, resp)); env.Error.Code != c.wantCode {
			t.Errorf("%s: got code %q, want %q", c.name, env.Error.Code, c.wantCode)
		}
	}
	// a check on a missing Source is a JSON 404
	resp := ts.do(t, "POST", "/api/v1/sources/999/check", `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("check missing: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "not_found" {
		t.Fatalf("check missing code: got %q", env.Error.Code)
	}
	// a non-numeric id is not found, not an internal error
	resp = ts.do(t, "GET", "/api/v1/sources/abc", "", "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-numeric id: got %d", resp.StatusCode)
	}
	// list before initialization is not_initialized
	ts2 := startServer(t)
	resp = ts2.do(t, "GET", "/api/v1/sources", "", "", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("list uninitialized: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "not_initialized" {
		t.Fatalf("list uninitialized code: got %q", env.Error.Code)
	}
}

func TestSourcesRESTSecurity(t *testing.T) {
	ts := startServer(t)
	resp := ts.do(t, "POST", "/api/v1/setup", `{}`, "application/json", "", "")
	resp.Body.Close()

	// mutations require the exact loopback origin
	resp = ts.do(t, "POST", "/api/v1/sources", `{"location": "/tmp/x"}`, "application/json", "", "http://evil.example")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad origin: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	// Host spoofing is rejected
	resp = ts.do(t, "POST", "/api/v1/sources", `{"location": "/tmp/x"}`, "application/json", "evil.example", "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad host: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
}
