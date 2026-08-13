package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// createAPIGroup registers one Group through the REST boundary and returns
// its id.
func createAPIGroup(t *testing.T, ts *testServer, name string) int64 {
	t.Helper()
	resp := ts.do(t, "POST", "/api/v1/groups", fmt.Sprintf(`{"name": %q}`, name), "application/json", "", "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create group: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var created groupJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &created); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

func decodeGroupView(t *testing.T, body string) groupViewJSON {
	t.Helper()
	var view groupViewJSON
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return view
}

// TestGroupsRESTLifecycle covers create, list, detail, membership add and
// remove, and the stable error mapping through the REST boundary.
func TestGroupsRESTLifecycle(t *testing.T) {
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
	skillIDs := []int64{}
	for _, s := range decodeSkillList(t, readBodyOf(t, ts, "/api/v1/skills")) {
		skillIDs = append(skillIDs, s.ID)
	}
	if len(skillIDs) != 2 {
		t.Fatalf("skills: %v", skillIDs)
	}

	// create + list
	id := createAPIGroup(t, ts, "eng")
	resp = ts.do(t, "GET", "/api/v1/groups", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list groups: got %d", resp.StatusCode)
	}
	var envelope struct {
		Items []groupSummaryJSON `json:"items"`
		Total int                `json:"total"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Total != 1 || envelope.Items[0].Name != "eng" || envelope.Items[0].MemberCount != 0 {
		t.Fatalf("groups list: %+v", envelope)
	}

	// duplicate name conflicts
	resp = ts.do(t, "POST", "/api/v1/groups", `{"name": "eng"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate group: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	// membership add is idempotent and returns the refreshed view
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/groups/%d/members", id),
		fmt.Sprintf(`{"skill_ids": [%d, %d]}`, skillIDs[0], skillIDs[1]), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add members: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	view := decodeGroupView(t, readBody(t, resp))
	if len(view.Members) != 2 {
		t.Fatalf("members: %+v", view.Members)
	}
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/groups/%d/members", id),
		fmt.Sprintf(`{"skill_ids": [%d]}`, skillIDs[0]), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-add member: got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// show
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/groups/%d", id), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("show group: got %d", resp.StatusCode)
	}
	view = decodeGroupView(t, readBody(t, resp))
	if view.ID != id || len(view.Members) != 2 || view.Members[0].Slug != "alpha" {
		t.Fatalf("group view: %+v", view)
	}

	// remove one member by skill id
	resp = ts.do(t, "DELETE", fmt.Sprintf("/api/v1/groups/%d/members/%d", id, skillIDs[0]), "", "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove member: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	view = decodeGroupView(t, readBody(t, resp))
	if len(view.Members) != 1 || view.Members[0].Slug != "beta" {
		t.Fatalf("members after removal: %+v", view.Members)
	}

	// error mapping: unknown group, unknown skill, malformed body
	resp = ts.do(t, "GET", "/api/v1/groups/999", "", "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing group: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/groups/%d/members", id), `{"skill_ids": [999]}`, "application/json", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing skill member: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ts.do(t, "POST", "/api/v1/groups", `{"name": }`, "application/json", "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	// non-numeric ids are not found
	resp = ts.do(t, "GET", "/api/v1/groups/abc", "", "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-numeric id: got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// readBodyOf issues a GET and returns the decoded body.
func readBodyOf(t *testing.T, ts *testServer, path string) string {
	t.Helper()
	resp := ts.do(t, "GET", path, "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: got %d", path, resp.StatusCode)
	}
	return readBody(t, resp)
}
