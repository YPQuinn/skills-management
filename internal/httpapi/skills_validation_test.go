package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

// TestSkillsRESTValidation covers strict JSON decode, request-level app
// errors through the existing mapping, and route isolation.
func TestSkillsRESTValidation(t *testing.T) {
	ts := readyServer(t, nil)
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	sourceID := addAPISource(t, ts, root, "one")

	cases := []struct {
		name, body, contentType string
		wantStatus              int
		wantCode                string
	}{
		{"invalid json", `{`, "application/json", http.StatusBadRequest, "bad_request"},
		{"unknown field", `{"source_id": 1, "bogus": 1}`, "application/json", http.StatusBadRequest, "bad_request"},
		{"trailing data", `{} {}`, "application/json", http.StatusBadRequest, "bad_request"},
		{"non-object body", `[]`, "application/json", http.StatusBadRequest, "bad_request"},
		{"missing source id", `{"selectors": [{"relative_dir": "skills/alpha"}]}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"all with selectors", `{"source_id": 1, "all": true, "selectors": [{"relative_dir": "skills/alpha"}]}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"no selectors", `{"source_id": 1}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"both relative_dir and name", `{"source_id": 1, "selectors": [{"relative_dir": "skills/alpha", "name": "alpha"}]}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"empty selector", `{"source_id": 1, "selectors": [{}]}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"selector wrong type", `{"source_id": 1, "selectors": [{"relative_dir": 5}]}`, "application/json", http.StatusBadRequest, "bad_request"},
		{"invalid slug", `{"source_id": 1, "selectors": [{"relative_dir": "skills/alpha", "slug": "Bad Slug"}]}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"explicit empty slug", `{"source_id": 1, "selectors": [{"relative_dir": "skills/alpha", "slug": ""}]}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"null body", `null`, "application/json", http.StatusBadRequest, "bad_request"},
		{"missing entry", `{"source_id": 1, "selectors": [{"relative_dir": "skills/missing"}]}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"no content type", `{}`, "", http.StatusUnsupportedMediaType, "unsupported_media_type"},
	}
	for _, c := range cases {
		resp := ts.do(t, "POST", "/api/v1/skills/import", c.body, c.contentType, "", "")
		if resp.StatusCode != c.wantStatus {
			t.Errorf("%s: got %d, want %d (body %s)", c.name, resp.StatusCode, c.wantStatus, readBody(t, resp))
			continue
		}
		if env := decodeError(t, readBody(t, resp)); env.Error.Code != c.wantCode {
			t.Errorf("%s: got code %q, want %q", c.name, env.Error.Code, c.wantCode)
		}
	}

	// every rejected request above must not have mutated the Store
	resp := ts.do(t, "GET", "/api/v1/skills", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list after rejections: got %d", resp.StatusCode)
	}
	if items := decodeSkillList(t, readBody(t, resp)); len(items) != 0 {
		t.Fatalf("rejected imports must not mutate the Store: %+v", items)
	}

	// an unknown Source id is not found, not an internal error
	resp = ts.do(t, "POST", "/api/v1/skills/import",
		fmt.Sprintf(`{"source_id": %d, "selectors": [{"relative_dir": "skills/alpha"}]}`, sourceID+99),
		"application/json", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown source: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "not_found" {
		t.Fatalf("unknown source code: got %q", env.Error.Code)
	}

	// detail route isolation: non-numeric and missing ids are JSON 404
	for _, path := range []string{"/api/v1/skills/abc", "/api/v1/skills/999", "/api/v1/skills/import"} {
		resp := ts.do(t, "GET", path, "", "", "", "")
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s: got %d, body %s", path, resp.StatusCode, readBody(t, resp))
		}
		if env := decodeError(t, readBody(t, resp)); env.Error.Code != "not_found" {
			t.Fatalf("GET %s code: got %q", path, env.Error.Code)
		}
	}

	// an uninitialized installation reports not_initialized for reads
	ts2 := startServer(t)
	resp = ts2.do(t, "GET", "/api/v1/skills", "", "", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("list uninitialized: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "not_initialized" {
		t.Fatalf("list uninitialized code: got %q", env.Error.Code)
	}
}
