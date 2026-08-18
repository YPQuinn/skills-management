package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupRecoverStoreRebuildsUnboundSkills(t *testing.T) {
	ts := startServer(t)
	resp := ts.do(t, "POST", "/api/v1/setup", `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: %d %s", resp.StatusCode, readBody(t, resp))
	}
	cfg, err := ts.bm.Config()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg.StorePath, "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: Alpha\ndescription: desc\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.StorePath, ".skillctl", "previous", "4"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cfg.StateDBPath); err != nil {
		t.Fatal(err)
	}

	resp = ts.do(t, "POST", "/api/v1/setup", `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("setup without recover: %d", resp.StatusCode)
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "state_missing" {
		t.Fatalf("setup without recover: %+v", env)
	}

	resp = ts.do(t, "POST", "/api/v1/setup", `{"recover_store": true, "store_path": "/tmp/other"}`, "application/json", "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("recover with store_path: %d %s", resp.StatusCode, readBody(t, resp))
	}

	resp = ts.do(t, "POST", "/api/v1/setup", `{"recover_store": true}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recover setup: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var got struct {
		State             string                        `json:"state"`
		Recovered         []struct{ Slug, Name string } `json:"recovered"`
		PreservedInternal []string                      `json:"preserved_internal"`
		Unrecoverable     []string                      `json:"unrecoverable"`
	}
	body := readBody(t, resp)
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode recover: %v\n%s", err, body)
	}
	if got.State != "ready" || len(got.Recovered) != 1 || got.Recovered[0].Slug != "alpha" {
		t.Fatalf("recover body: %+v", got)
	}
	if len(got.PreservedInternal) != 1 || got.PreservedInternal[0] != ".skillctl/previous/4" {
		t.Fatalf("preserved: %v", got.PreservedInternal)
	}
	if len(got.Unrecoverable) == 0 {
		t.Fatal("unrecoverable missing")
	}

	resp = ts.do(t, "GET", "/api/v1/skills", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list skills: %d", resp.StatusCode)
	}
	var skills struct {
		Items []struct {
			Slug       string    `json:"slug"`
			SyncStatus string    `json:"sync_status"`
			Binding    *struct{} `json:"binding"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &skills); err != nil {
		t.Fatal(err)
	}
	if skills.Total != 1 || skills.Items[0].SyncStatus != "unbound" || skills.Items[0].Binding != nil {
		t.Fatalf("skills after recover: %+v", skills)
	}
}

func TestSetupRecoverStoreRefusesUninitialized(t *testing.T) {
	ts := startServer(t)
	resp := ts.do(t, "POST", "/api/v1/setup", `{"recover_store": true}`, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("recover uninitialized: %d %s", resp.StatusCode, readBody(t, resp))
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "not_initialized" {
		t.Fatalf("code: %q", env.Error.Code)
	}
}
