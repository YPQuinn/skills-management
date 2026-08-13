package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
)

// TestImportImpactREST pins that the REST import result carries the
// replace-impact preview once Groups and Targets exist.
func TestImportImpactREST(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	ts := readyServer(t, nil)
	rootA := t.TempDir()
	writeAPISkill(t, rootA, "alpha")
	srcA := addAPISource(t, ts, rootA, "a")
	resp := ts.do(t, "POST", "/api/v1/skills/import", fmt.Sprintf(`{"source_id": %d, "all": true}`, srcA), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	alpha := decodeSkillList(t, readBodyOf(t, ts, "/api/v1/skills"))[0]

	groupID := createAPIGroup(t, ts, "eng")
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/groups/%d/members", groupID),
		fmt.Sprintf(`{"skill_ids": [%d]}`, alpha.ID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add member: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ts.do(t, "POST", "/api/v1/targets", `{"path": "~/skills"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register target: got %d", resp.StatusCode)
	}
	targetID := decodeTargetView(t, readBody(t, resp)).ID
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/assignments", targetID),
		fmt.Sprintf(`{"kind": "group", "group_id": %d}`, groupID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assign group: got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// a same-named entry from a second Source claims the slug
	rootB := t.TempDir()
	writeAPISkill(t, rootB, "alpha")
	srcB := addAPISource(t, ts, rootB, "b")
	resp = ts.do(t, "POST", "/api/v1/skills/import", fmt.Sprintf(`{"source_id": %d, "selectors": [{"relative_dir": "skills/alpha"}]}`, srcB), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("conflict import: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var result importSkillsResultJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &result); err != nil {
		t.Fatal(err)
	}
	item := result.Items[0]
	if item.Status != "skipped_conflict" || item.Impact == nil {
		t.Fatalf("conflict item: %+v", item)
	}
	if len(item.Impact.Groups) != 1 || item.Impact.Groups[0].Name != "eng" {
		t.Fatalf("impact groups: %+v", item.Impact.Groups)
	}
	if len(item.Impact.Targets) != 1 || item.Impact.Targets[0].Name == "" ||
		item.Impact.Targets[0].Direct || len(item.Impact.Targets[0].Groups) != 1 {
		t.Fatalf("impact targets: %+v", item.Impact.Targets)
	}
	if item.Impact.Targets[0].Groups[0].Name != "eng" {
		t.Fatalf("impact target groups: %+v", item.Impact.Targets[0].Groups)
	}
}
