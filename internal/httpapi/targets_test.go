package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
)

func decodeTargetView(t *testing.T, body string) targetViewJSON {
	t.Helper()
	var view targetViewJSON
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return view
}

func decodeAssignment(t *testing.T, body string) assignmentJSON {
	t.Helper()
	var as assignmentJSON
	if err := json.Unmarshal([]byte(body), &as); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return as
}

// TestTargetsRESTLifecycle covers registration, dedupe, assignments,
// desired-set expansion, unassignment, and the stable error mapping.
func TestTargetsRESTLifecycle(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	ts := readyServer(t, nil)
	root := t.TempDir()
	writeAPISkill(t, root, "alpha")
	writeAPISkill(t, root, "beta")
	srcID := addAPISource(t, ts, root, "local")
	resp := ts.do(t, "POST", "/api/v1/skills/import", fmt.Sprintf(`{"source_id": %d, "all": true}`, srcID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
	skills := decodeSkillList(t, readBodyOf(t, ts, "/api/v1/skills"))
	alphaID, betaID := skills[0].ID, skills[1].ID

	// register a custom Target; the same physical path returns it again
	resp = ts.do(t, "POST", "/api/v1/targets", `{"path": "~/skills", "name": "mine"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register target: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	view := decodeTargetView(t, readBody(t, resp))
	if view.Adapter != "custom" || view.Scope != "custom" || view.Path != filepath.Join(home, "skills") {
		t.Fatalf("target view: %+v", view)
	}
	targetID := view.ID
	resp = ts.do(t, "POST", "/api/v1/targets", `{"path": "~/skills", "name": "alias"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register same path: got %d", resp.StatusCode)
	}
	if again := decodeTargetView(t, readBody(t, resp)); again.ID != targetID {
		t.Fatalf("same path must return the existing Target: %d != %d", again.ID, targetID)
	}

	// register a built-in user Target
	resp = ts.do(t, "POST", "/api/v1/targets", `{"adapter": "pi", "scope": "user"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register pi: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	piView := decodeTargetView(t, readBody(t, resp))
	if piView.Path != filepath.Join(home, ".pi", "agent", "skills") ||
		len(piView.CompatibleAdapters) != 1 || piView.CompatibleAdapters[0] != "pi" {
		t.Fatalf("pi view: %+v", piView)
	}

	// list
	resp = ts.do(t, "GET", "/api/v1/targets", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list targets: got %d", resp.StatusCode)
	}
	var list struct {
		Items []targetJSON `json:"items"`
		Total int          `json:"total"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 {
		t.Fatalf("targets list: %+v", list)
	}
	for _, item := range list.Items {
		if item.LastResult != "" || item.Stale {
			t.Fatalf("fresh Target list must omit distribution health: %+v", item)
		}
	}

	// assign a Skill and then the same Skill again (idempotent)
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/assignments", targetID),
		fmt.Sprintf(`{"kind": "skill", "skill_id": %d}`, alphaID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assign skill: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	as := decodeAssignment(t, readBody(t, resp))
	if as.Kind != "skill" || as.Skill == nil || as.Skill.Slug != "alpha" {
		t.Fatalf("assignment: %+v", as)
	}
	assignmentID := as.ID
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/assignments", targetID),
		fmt.Sprintf(`{"kind": "skill", "skill_id": %d}`, alphaID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-assign: got %d", resp.StatusCode)
	}
	if again := decodeAssignment(t, readBody(t, resp)); again.ID != assignmentID {
		t.Fatalf("re-assign must reuse the Assignment: %d != %d", again.ID, assignmentID)
	}

	// a Group Assignment expands the desired set
	resp = ts.do(t, "POST", "/api/v1/groups", `{"name": "eng"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create group: got %d", resp.StatusCode)
	}
	var group groupJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &group); err != nil {
		t.Fatal(err)
	}
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/groups/%d/members", group.ID),
		fmt.Sprintf(`{"skill_ids": [%d, %d]}`, alphaID, betaID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add members: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/assignments", targetID),
		fmt.Sprintf(`{"kind": "group", "group_id": %d}`, group.ID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assign group: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	// show carries the expanded desired set with explanations
	view = decodeTargetView(t, readBodyOf(t, ts, fmt.Sprintf("/api/v1/targets/%d", targetID)))
	if len(view.DirectSkills) != 1 || len(view.Groups) != 1 || len(view.DesiredSkills) != 2 {
		t.Fatalf("target view: %+v", view)
	}
	var alphaDesired *desiredSkillJSON
	for i := range view.DesiredSkills {
		if view.DesiredSkills[i].Slug == "alpha" {
			alphaDesired = &view.DesiredSkills[i]
		}
	}
	if alphaDesired == nil || len(alphaDesired.Reasons) != 2 {
		t.Fatalf("alpha desired: %+v", alphaDesired)
	}

	// DELETE by Assignment id refreshes the view
	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/targets/%d/assignments/%d", targetID, assignmentID), "", "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete assignment: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	view = decodeTargetView(t, readBody(t, resp))
	if len(view.DirectSkills) != 0 || len(view.DesiredSkills) != 2 {
		t.Fatalf("view after delete: %+v", view)
	}
	// deleting it again is not_found
	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/targets/%d/assignments/%d", targetID, assignmentID), "", "application/json", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing assignment: got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// error mapping
	resp = ts.do(t, "POST", "/api/v1/targets", `{"path": "/"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("filesystem root: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ts.do(t, "POST", "/api/v1/targets", `{"adapter": "nope", "scope": "user"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown adapter: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ts.do(t, "GET", "/api/v1/targets/999", "", "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing target: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/assignments", targetID), `{"kind": "group"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing subject: got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
