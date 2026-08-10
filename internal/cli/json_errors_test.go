package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

// runRoot executes one full command tree through executeWith with separated
// stdout and stderr buffers, mirroring the real binary entry point.
func runRoot(t *testing.T, bm *bootstrap.Manager, args ...string) (string, string, error) {
	t.Helper()
	cmd := newRootCmd(bm)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err := executeWith(context.Background(), cmd)
	return out.String(), errBuf.String(), err
}

func TestSourceJSONErrors(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	if _, _, err := runRoot(t, bm, "source", "add", root, "--name", "dup"); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	writeCLISkill(t, other, "beta")

	cases := []struct {
		name     string
		args     []string
		wantCode string
		wantExit int
	}{
		{"not found", []string{"source", "show", "missing", "--json"}, "not_found", 1},
		{"invalid argument", []string{"source", "show", "--json"}, "invalid_argument", 2},
		{"conflict", []string{"source", "add", other, "--name", "dup", "--json"}, "conflict", 1},
	}
	for _, c := range cases {
		stdout, stderr, err := runRoot(t, bm, c.args...)
		if err == nil {
			t.Fatalf("%s: want an error", c.name)
		}
		if got := ExitCode(err); got != c.wantExit {
			t.Errorf("%s: exit %d, want %d", c.name, got, c.wantExit)
		}
		var res struct {
			Error struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Details map[string]any `json:"details"`
			} `json:"error"`
		}
		if jerr := json.Unmarshal([]byte(stdout), &res); jerr != nil {
			t.Fatalf("%s: stdout is not exactly one JSON value: %v\n%s", c.name, jerr, stdout)
		}
		if res.Error.Code != c.wantCode || res.Error.Message == "" {
			t.Errorf("%s: JSON error %+v, want code %q with a message", c.name, res.Error, c.wantCode)
		}
		if res.Error.Details == nil || len(res.Error.Details) != 0 {
			t.Errorf("%s: details must be an empty object, got %v", c.name, res.Error.Details)
		}
		if !strings.Contains(stderr, err.Error()) {
			t.Errorf("%s: stderr must carry the human message, got %q", c.name, stderr)
		}
	}
}

func TestSourceJSONErrorUninitialized(t *testing.T) {
	bm := bootstrap.New(filepath.Join(t.TempDir(), ".skillctl", "config.toml"))
	stdout, stderr, err := runRoot(t, bm, "source", "list", "--json")
	if err == nil {
		t.Fatal("uninitialized list: want an error")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("exit: got %d, want 1", ExitCode(err))
	}
	var res struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &res); jerr != nil {
		t.Fatalf("stdout is not one JSON value: %v\n%s", jerr, stdout)
	}
	if res.Error.Code != app.CodeNotInitialized {
		t.Fatalf("JSON code: got %q, want %q", res.Error.Code, app.CodeNotInitialized)
	}
	if res.Error.Details == nil || len(res.Error.Details) != 0 {
		t.Fatalf("details must be an empty object: %v", res.Error.Details)
	}
	if !strings.Contains(stderr, "not initialized") {
		t.Fatalf("stderr must carry the human message: %q", stderr)
	}
}

func TestSourceJSONFalseStaysHuman(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	stdout, stderr, err := runRoot(t, bm, "source", "show", "missing", "--json=false")
	if err == nil {
		t.Fatal("show missing: want an error")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("exit: got %d, want 1", ExitCode(err))
	}
	if stdout != "" {
		t.Fatalf("--json=false must not emit JSON on stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "not found") {
		t.Fatalf("stderr must carry the human message: %q", stderr)
	}
}

func TestPrintJSONErrorUntyped(t *testing.T) {
	var buf bytes.Buffer
	if err := printJSONError(&buf, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	var res jsonErrorResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Error.Code != app.CodeInternal || res.Error.Message != "boom" {
		t.Fatalf("untyped error: %+v", res.Error)
	}
	if res.Error.Details == nil || len(res.Error.Details) != 0 {
		t.Fatalf("details must be an empty object: %v", res.Error.Details)
	}
}
