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

func decodeDistributionResult(t *testing.T, body string) distributionResultJSON {
	t.Helper()
	var res distributionResultJSON
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return res
}

func decodeDistributionStatus(t *testing.T, body string) distributionStatusJSON {
	t.Helper()
	var st distributionStatusJSON
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return st
}

// TestDistributionRESTLifecycle covers the Target distribution routes and
// shapes: inspect, dry-run planning, reconciliation, conflict preservation,
// adoption, and the stable error mapping for ineligible adoption.
func TestDistributionRESTLifecycle(t *testing.T) {
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
		t.Fatalf("import: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
	skills := decodeSkillList(t, readBodyOf(t, ts, "/api/v1/skills"))
	alphaID := skills[0].ID

	resp = ts.do(t, "POST", "/api/v1/targets", `{"path": "~/skills", "name": "mine"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	targetID := decodeTargetView(t, readBody(t, resp)).ID
	resp.Body.Close()
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/assignments", targetID),
		fmt.Sprintf(`{"kind": "skill", "skill_id": %d}`, alphaID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assign: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	// inspect observes the desired Skill as missing
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/inspect", targetID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inspect: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	st := decodeDistributionStatus(t, readBody(t, resp))
	if st.State != "missing" || len(st.Items) != 1 || st.Items[0].Observed != "missing" {
		t.Fatalf("inspect status: %+v", st)
	}
	resp.Body.Close()

	// the dry run predicts the create without mutating
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/distribute", targetID), `{"dry_run": true}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dry run: got %d", resp.StatusCode)
	}
	dry := decodeDistributionResult(t, readBody(t, resp))
	if dry.DryRun != true || dry.Outcome != "succeeded" || dry.Items[0].Result != "created" {
		t.Fatalf("dry run: %+v", dry)
	}
	resp.Body.Close()
	link := filepath.Join(home, "skills", "alpha")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("dry run must not mutate")
	}

	// distribution creates the link
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/distribute", targetID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("distribute: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	res := decodeDistributionResult(t, readBody(t, resp))
	if res.Outcome != "succeeded" || res.Summary.Created != 1 {
		t.Fatalf("distribute: %+v", res)
	}
	resp.Body.Close()
	if raw, err := os.Readlink(link); err != nil || !filepath.IsAbs(raw) {
		t.Fatalf("link: %q, %v", raw, err)
	}

	// the Target detail carries the stored Distribution Status
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/targets/%d", targetID), "", "application/json", "", "")
	view := decodeTargetView(t, readBody(t, resp))
	resp.Body.Close()
	if view.Distribution == nil || len(view.Distribution.Items) != 1 || view.Distribution.Items[0].Observed != "linked" {
		t.Fatalf("stored distribution: %+v", view.Distribution)
	}

	// a foreign entry blocks as a conflict and adopt reports the exact shape
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/distribute", targetID), `{}`, "application/json", "", "")
	blocked := decodeDistributionResult(t, readBody(t, resp))
	resp.Body.Close()
	if blocked.Outcome != "blocked" || blocked.Items[0].Result != "blocked_conflict" {
		t.Fatalf("blocked: %+v", blocked)
	}
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/adopt", targetID),
		fmt.Sprintf(`{"skill_id": %d}`, alphaID), "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("file adopt: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	// replacing the entry with the correct symlink allows adoption
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	a, err := ts.bm.App()
	if err != nil {
		t.Fatal(err)
	}
	storeRoot, err := filepath.EvalSymlinks(a.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(storeRoot, "alpha"), link); err != nil {
		t.Fatal(err)
	}
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/adopt", targetID),
		fmt.Sprintf(`{"skill_id": %d}`, alphaID), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("adopt: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var ar adoptResultJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &ar); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ar.Result != "adopted" || ar.Slug != "alpha" {
		t.Fatalf("adopt result: %+v", ar)
	}

	// a malformed distribute body is a 400 before the request runs
	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/targets/%d/distribute", targetID), `{`, "application/json", "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	resp.Body.Close()
	if !strings.Contains(body, "invalid_argument") && !strings.Contains(body, "invalid JSON") {
		t.Fatalf("malformed body: %q", body)
	}
}
