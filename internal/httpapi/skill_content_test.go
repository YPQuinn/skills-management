package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestSkillContentREST(t *testing.T) {
	ts := startServer(t)
	resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	_, skillID := addAPIAndImport(t, ts, "alpha")
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/skills/%d/content", skillID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("content: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var got skillContentJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &got); err != nil {
		t.Fatal(err)
	}
	if got.SkillID != skillID || got.Slug != "alpha" || got.Path != "SKILL.md" {
		t.Fatalf("content: %+v", got)
	}
	if got.Frontmatter == nil {
		t.Fatal("frontmatter must be an array, not null")
	}
	if len(got.Frontmatter) != 2 ||
		got.Frontmatter[0] != (skillFrontmatterFieldJSON{Key: "name", Value: "alpha"}) ||
		got.Frontmatter[1] != (skillFrontmatterFieldJSON{Key: "description", Value: "desc"}) {
		t.Fatalf("frontmatter: %+v", got.Frontmatter)
	}
	if got.Body == "" {
		t.Fatal("body must be non-empty")
	}

	resp = ts.do(t, "GET", "/api/v1/skills/999999/content", "", "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "not_found" {
		t.Fatalf("unknown id code: got %q", env.Error.Code)
	}
}
