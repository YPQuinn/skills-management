package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCLISourceJSONErrorsSeparateProcess proves the --json failure contract
// through the real binary: stdout carries exactly one parseable JSON error
// result, human text stays on stderr, and exit codes are unchanged.
func TestCLISourceJSONErrorsSeparateProcess(t *testing.T) {
	home := t.TempDir()

	// uninitialized installation: blocked use case, exit 1, JSON on stdout
	stdout, stderr, code := runOut(t, home, "source", "list", "--json")
	if code != 1 {
		t.Fatalf("uninitialized list --json: exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var res struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("stdout is not exactly one JSON value: %v\n%s", err, stdout)
	}
	if res.Error.Code != "not_initialized" || res.Error.Message == "" {
		t.Fatalf("JSON error: %+v", res.Error)
	}
	if res.Error.Details == nil || len(res.Error.Details) != 0 {
		t.Fatalf("details must be an empty object: %v", res.Error.Details)
	}
	if !strings.Contains(stderr, "not initialized") {
		t.Fatalf("human message must stay on stderr: %q", stderr)
	}

	// invalid argument keeps exit 2 with JSON on stdout
	stdout, _, code = runOut(t, home, "source", "show", "--json")
	if code != 2 {
		t.Fatalf("show --json without id: exit %d, want 2\nstdout: %s", code, stdout)
	}
	var invalid struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &invalid); err != nil {
		t.Fatalf("stdout is not one JSON value: %v\n%s", err, stdout)
	}
	if invalid.Error.Code != "invalid_argument" {
		t.Fatalf("JSON code: got %q, want invalid_argument", invalid.Error.Code)
	}

	// --json=false keeps the human contract: empty stdout, exit unchanged
	stdout, stderr, code = runOut(t, home, "source", "list", "--json=false")
	if code != 1 {
		t.Fatalf("list --json=false: exit %d, want 1", code)
	}
	if stdout != "" {
		t.Fatalf("--json=false must not write JSON to stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "not initialized") {
		t.Fatalf("--json=false human message: %q", stderr)
	}
}
