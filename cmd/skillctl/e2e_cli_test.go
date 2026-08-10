// Package main contains the separate-process end-to-end tests for skillctl.
// They build a temporary binary once per test run, isolate HOME per child
// process, use port 0 everywhere, and never sleep-poll: readiness is
// signalled by the URL line on stdout (printed after the listener is bound,
// so connections queue in the kernel backlog).
package main

import (
	"os"
	"path/filepath"
	"skillctl/internal/lock"
	"strings"
	"testing"
)

func TestCLIInitRestartStateMissingInvalid(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "custom-store")

	out, code := run(t, home, "init", "--store", store)
	if code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(store); err != nil {
		t.Fatalf("custom Store was not created: %v", err)
	}
	cfgPath := filepath.Join(home, ".skillctl", "config.toml")
	cfg, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if !strings.Contains(string(cfg), store) {
		t.Errorf("config does not record the custom Store path:\n%s", cfg)
	}

	// restart: a fresh process parses the configuration and reports ready
	out, code = run(t, home, "status")
	if code != 0 || !strings.Contains(out, "Status: ready") {
		t.Fatalf("status after restart: exit %d\n%s", code, out)
	}

	// state_missing
	if err := os.Remove(filepath.Join(home, ".skillctl", "state.db")); err != nil {
		t.Fatal(err)
	}
	out, code = run(t, home, "status")
	if code != 0 || !strings.Contains(out, "Status: state_missing") {
		t.Fatalf("status with missing state: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "init")
	if code != 1 || !strings.Contains(out, "state database is missing") {
		t.Fatalf("init on state_missing: exit %d\n%s", code, out)
	}

	// invalid configuration is a validation failure: exit 2 with the error
	if err := os.WriteFile(cfgPath, []byte("this is [not toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = run(t, home, "status")
	if code != 2 || !strings.Contains(out, "invalid configuration") {
		t.Fatalf("status on invalid config: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "init")
	if code != 2 {
		t.Fatalf("init on invalid config: exit %d, want 2\n%s", code, out)
	}

	// argument validation failures exit 2
	out, code = run(t, home, "init", "--store", "relative")
	if code != 2 || !strings.Contains(out, "absolute") {
		t.Fatalf("init with relative store: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "bogus")
	if code != 2 {
		t.Fatalf("unknown command: exit %d, want 2\n%s", code, out)
	}
	out, code = run(t, home, "init", "extra")
	if code != 2 {
		t.Fatalf("unexpected argument: exit %d, want 2\n%s", code, out)
	}
}

func TestCLIVersion(t *testing.T) {
	out, code := run(t, t.TempDir(), "--version")
	if code != 0 {
		t.Fatalf("--version: exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "dev") || !strings.Contains(out, "unknown") {
		t.Fatalf("--version must report the development version and commit defaults:\n%s", out)
	}
}

func TestCLILockContention(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, ".skillctl")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}

	held, err := lock.TryExclusive(filepath.Join(base, "skillctl.lock"))
	if err != nil {
		t.Fatal(err)
	}

	out, code := run(t, home, "init")
	if code != 1 || !strings.Contains(out, "another skillctl process") {
		t.Fatalf("init under held writer lock: exit %d\n%s", code, out)
	}

	if err := held.Unlock(); err != nil {
		t.Fatal(err)
	}
	out, code = run(t, home, "init")
	if code != 0 {
		t.Fatalf("init after releasing the lock: exit %d\n%s", code, out)
	}
}
