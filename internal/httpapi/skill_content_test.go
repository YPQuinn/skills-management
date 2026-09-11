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

// disableModelInvocationField returns the disable-model-invocation value from
// a content JSON, and whether the field is present at all.
func disableModelInvocationField(c skillContentJSON) (value string, present bool) {
	for _, f := range c.Frontmatter {
		if f.Key == "disable-model-invocation" {
			return f.Value, true
		}
	}
	return "", false
}

func TestSetSkillModelInvocationREST(t *testing.T) {
	ts := startServer(t)
	if resp := ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: %d %s", resp.StatusCode, readBody(t, resp))
	}
	_, skillID := addAPIAndImport(t, ts, "alpha")

	// Turning it on adds the frontmatter field and diverges the Store from
	// the accepted Source, so the Skill evaluates to store_changed.
	resp := ts.do(t, "PUT", fmt.Sprintf("/api/v1/skills/%d/model-invocation", skillID),
		`{"disabled": true}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enable: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var content skillContentJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &content); err != nil {
		t.Fatal(err)
	}
	if v, ok := disableModelInvocationField(content); !ok || v != "true" {
		t.Fatalf("after enable, frontmatter: %+v", content.Frontmatter)
	}
	var skill skillJSON
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/skills/%d", skillID), "", "", "", "")
	if err := json.Unmarshal([]byte(readBody(t, resp)), &skill); err != nil {
		t.Fatal(err)
	}
	if skill.SyncStatus != "store_changed" {
		t.Fatalf("after enable, sync status: %q", skill.SyncStatus)
	}

	// Turning it off removes the field again.
	resp = ts.do(t, "PUT", fmt.Sprintf("/api/v1/skills/%d/model-invocation", skillID),
		`{"disabled": false}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable: %d %s", resp.StatusCode, readBody(t, resp))
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &content); err != nil {
		t.Fatal(err)
	}
	if _, ok := disableModelInvocationField(content); ok {
		t.Fatalf("after disable, field must be absent: %+v", content.Frontmatter)
	}

	resp = ts.do(t, "PUT", "/api/v1/skills/999999/model-invocation",
		`{"disabled": true}`, "application/json", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id: %d %s", resp.StatusCode, readBody(t, resp))
	}
}
