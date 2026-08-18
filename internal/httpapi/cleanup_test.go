package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupRESTSourceDetachAndSkillRebind(t *testing.T) {
	ts := readyServer(t, nil)
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	srcID := addAPISource(t, ts, root, "local")
	resp := ts.do(t, "POST", "/api/v1/skills/import", fmt.Sprintf(`{"source_id": %d, "all": true}`, srcID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
	skills := decodeSkillList(t, readBodyOf(t, ts, "/api/v1/skills"))
	alphaID := skills[0].ID

	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/sources/%d/deletion", srcID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var preview struct {
		RequiresDetach bool `json:"requires_detach"`
		BoundSkills    []struct {
			Slug string `json:"slug"`
		} `json:"bound_skills"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.RequiresDetach || len(preview.BoundSkills) != 1 {
		t.Fatalf("preview: %+v", preview)
	}

	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/sources/%d", srcID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("delete without detach: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/skills/%d/detach", alphaID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detach: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var detached skillJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &detached); err != nil {
		t.Fatal(err)
	}
	if detached.Binding != nil || detached.SyncStatus != "unbound" {
		t.Fatalf("detached: %+v", detached)
	}

	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/sources/%d", srcID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete unbound source: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	rootB := t.TempDir()
	writeAPISkill(t, rootB, "alpha")
	srcB := addAPISource(t, ts, rootB, "other")
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/skills/%d/rebind", alphaID),
		fmt.Sprintf(`{"source_id": %d, "relative_dir": "skills/alpha"}`, srcB), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rebind: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var rebound struct {
		Identical bool      `json:"identical"`
		Skill     skillJSON `json:"skill"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &rebound); err != nil {
		t.Fatal(err)
	}
	if !rebound.Identical || rebound.Skill.SyncStatus != "in_sync" {
		t.Fatalf("rebind: %+v", rebound)
	}
}

func TestCleanupRESTSkillAndTargetDelete(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	ts := readyServer(t, nil)
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	srcID := addAPISource(t, ts, root, "local")
	resp := ts.do(t, "POST", "/api/v1/skills/import", fmt.Sprintf(`{"source_id": %d, "all": true}`, srcID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	alphaID := decodeSkillList(t, readBodyOf(t, ts, "/api/v1/skills"))[0].ID

	resp = ts.do(t, "POST", "/api/v1/targets", `{"path": "~/skills", "name": "mine"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	targetID := decodeTargetView(t, readBody(t, resp)).ID
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/assignments", targetID),
		fmt.Sprintf(`{"kind": "skill", "skill_id": %d}`, alphaID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assign: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/distribute", targetID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("distribute: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/skills/%d", alphaID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("delete referenced: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/skills/%d/deletion", alphaID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("skill preview: got %d", resp.StatusCode)
	}
	var skillPreview struct {
		Referenced bool `json:"referenced"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &skillPreview); err != nil || !skillPreview.Referenced {
		t.Fatalf("skill preview: %+v, %v", skillPreview, err)
	}

	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/targets/%d", targetID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete target: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
	if _, err := os.Lstat(filepath.Join(home, "skills")); err != nil {
		t.Fatalf("container must remain: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, "skills", "alpha")); !os.IsNotExist(err) {
		t.Fatalf("managed link must be gone: %v", err)
	}

	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/skills/%d", alphaID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete unreferenced skill: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
}

func TestCleanupRESTGroupUnassign(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	ts := readyServer(t, nil)
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	srcID := addAPISource(t, ts, root, "local")
	resp := ts.do(t, "POST", "/api/v1/skills/import", fmt.Sprintf(`{"source_id": %d, "all": true}`, srcID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	alphaID := decodeSkillList(t, readBodyOf(t, ts, "/api/v1/skills"))[0].ID
	groupID := createAPIGroup(t, ts, "eng")
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/groups/%d/members", groupID),
		fmt.Sprintf(`{"skill_ids": [%d]}`, alphaID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add member: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
	resp = ts.do(t, "POST", "/api/v1/targets", `{"path": "~/skills", "name": "mine"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register: got %d", resp.StatusCode)
	}
	targetID := decodeTargetView(t, readBody(t, resp)).ID
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/assignments", targetID),
		fmt.Sprintf(`{"kind": "group", "group_id": %d}`, groupID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assign group: got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/groups/%d", groupID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("delete assigned group: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/groups/%d", groupID), `{"unassign": true}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete with unassign: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
}

func TestCleanupRESTRejectsBadBodies(t *testing.T) {
	ts := readyServer(t, nil)
	resp := ts.do(t, "DELETE", "/api/v1/sources/1", `[]`, "application/json", "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad source body: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ts.do(t, "POST", "/api/v1/skills/1/detach", ``, "application/json", "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty detach: got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
