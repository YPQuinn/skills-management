package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestStatusAndSetupTransitions(t *testing.T) {
	ts := startServer(t)

	resp := ts.do(t, "GET", "/api/v1/status", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	if st := decodeStatus(t, readBody(t, resp)); st.State != "uninitialized" {
		t.Fatalf("initial state: got %q", st.State)
	}

	// setup transitions the running manager without a restart
	resp = ts.do(t, "POST", "/api/v1/setup", `{"store_path": ""}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if st := decodeStatus(t, readBody(t, resp)); st.State != "ready" {
		t.Fatalf("setup response state: got %q", st.State)
	}
	if _, err := ts.bm.App(); err != nil {
		t.Fatalf("setup did not construct the app in-process: %v", err)
	}
	resp = ts.do(t, "GET", "/api/v1/status", "", "", "", "")
	if st := decodeStatus(t, readBody(t, resp)); st.State != "ready" {
		t.Fatalf("state after setup: got %q", st.State)
	}

	// second setup conflicts
	resp = ts.do(t, "POST", "/api/v1/setup", `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second setup: got %d", resp.StatusCode)
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "already_initialized" {
		t.Fatalf("second setup code: got %q", env.Error.Code)
	}

	// deleted state database reports state_missing and blocks setup
	if err := os.Remove(filepath.Join(filepath.Dir(ts.cfgPath), "state.db")); err != nil {
		t.Fatal(err)
	}
	resp = ts.do(t, "GET", "/api/v1/status", "", "", "", "")
	if st := decodeStatus(t, readBody(t, resp)); st.State != "state_missing" {
		t.Fatalf("state after db removal: got %q", st.State)
	}
	resp = ts.do(t, "POST", "/api/v1/setup", `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("setup on state_missing: got %d", resp.StatusCode)
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "state_missing" {
		t.Fatalf("setup on state_missing code: got %q", env.Error.Code)
	}
}

func TestSetupCustomStoreAndValidation(t *testing.T) {
	ts := startServer(t)

	custom := filepath.Join(t.TempDir(), "custom-store")
	resp := ts.do(t, "POST", "/api/v1/setup", fmt.Sprintf(`{"store_path": %q}`, custom), "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup custom store: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatalf("custom store not created: %v", err)
	}
	cfg, err := ts.bm.Config()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StorePath != custom {
		t.Fatalf("config store path: got %q, want %q", cfg.StorePath, custom)
	}
}

func TestSetupBadRequests(t *testing.T) {
	ts := startServer(t)
	cases := []struct {
		name, body, contentType string
		wantStatus              int
		wantCode                string
	}{
		{"empty body", "", "application/json", http.StatusBadRequest, "bad_request"},
		{"invalid json", `{`, "application/json", http.StatusBadRequest, "bad_request"},
		{"wrong type", `{"store_path": 5}`, "application/json", http.StatusBadRequest, "bad_request"},
		{"unknown field", `{"store_paths": "/x"}`, "application/json", http.StatusBadRequest, "bad_request"},
		{"relative store", `{"store_path": "relative"}`, "application/json", http.StatusBadRequest, "invalid_argument"},
		{"trailing data", `{} {}`, "application/json", http.StatusBadRequest, "bad_request"},
		{"text content type", `{}`, "text/plain", http.StatusUnsupportedMediaType, "unsupported_media_type"},
		{"no content type", `{}`, "", http.StatusUnsupportedMediaType, "unsupported_media_type"},
		{"missing asset content type", `{}`, "application/octet-stream", http.StatusUnsupportedMediaType, "unsupported_media_type"},
	}
	for _, c := range cases {
		resp := ts.do(t, "POST", "/api/v1/setup", c.body, c.contentType, "", "")
		if resp.StatusCode != c.wantStatus {
			t.Errorf("%s: got %d, want %d (body %s)", c.name, resp.StatusCode, c.wantStatus, readBody(t, resp))
			continue
		}
		if env := decodeError(t, readBody(t, resp)); env.Error.Code != c.wantCode {
			t.Errorf("%s: got code %q, want %q", c.name, env.Error.Code, c.wantCode)
		}
	}

	// charset parameter is accepted for mutations
	resp := ts.do(t, "POST", "/api/v1/setup", `{}`, "application/json; charset=utf-8", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup with charset: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}

	// reads do not require a content type
	resp = ts.do(t, "GET", "/api/v1/status", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status without content type: got %d", resp.StatusCode)
	}
}
func TestSetupInvalidConfig(t *testing.T) {
	ts := startServer(t)
	if err := os.MkdirAll(filepath.Dir(ts.cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ts.cfgPath, []byte("broken [toml"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := ts.do(t, "GET", "/api/v1/status", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status on invalid config: got %d", resp.StatusCode)
	}
	st := decodeStatus(t, readBody(t, resp))
	if st.State != "invalid" || st.Message == "" {
		t.Fatalf("invalid state response: %+v", st)
	}

	resp = ts.do(t, "POST", "/api/v1/setup", `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("setup on invalid config: got %d", resp.StatusCode)
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "invalid_config" {
		t.Fatalf("setup on invalid config code: got %q", env.Error.Code)
	}
}
