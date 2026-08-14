package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLISyncEndToEnd locks the separate-process synchronization journey
// (decision 09): modify Source content, observe source_changed,
// synchronize, rollback, and verify durable state across processes.
func TestCLISyncEndToEnd(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "demo")
	if out, code := run(t, home, "source", "add", root, "--name", "sync-local"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "sync-local", "--all"); code != 0 {
		t.Fatalf("skill import: exit %d\n%s", code, out)
	}

	// check evaluates the accepted state.
	out, code := run(t, home, "skill", "check", "demo")
	if code != 0 || !strings.Contains(out, `"demo": in_sync`) {
		t.Fatalf("skill check: exit %d\n%s", code, out)
	}

	// Modify Source content: the check observes source_changed.
	if err := os.WriteFile(filepath.Join(root, "skills", "demo", "notes.md"), []byte("upstream\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = run(t, home, "skill", "check", "demo")
	if code != 0 || !strings.Contains(out, `"demo": source_changed`) {
		t.Fatalf("skill check after change: exit %d\n%s", code, out)
	}

	// Synchronize: the Store receives the change and the process exits 0.
	out, code = run(t, home, "skill", "sync", "demo")
	if code != 0 || !strings.Contains(out, "updated") {
		t.Fatalf("skill sync: exit %d\n%s", code, out)
	}
	if data, err := os.ReadFile(filepath.Join(store, "demo", "notes.md")); err != nil || string(data) != "upstream\n" {
		t.Fatalf("store content: %q, %v", data, err)
	}

	// Rollback restores the previous snapshot and reports store_changed.
	out, code = run(t, home, "skill", "rollback", "demo")
	if code != 0 || !strings.Contains(out, "rolled_back") || !strings.Contains(out, "store_changed") {
		t.Fatalf("skill rollback: exit %d\n%s", code, out)
	}
	if _, err := os.Lstat(filepath.Join(store, "demo", "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("rolled-back content must not carry the upstream file: %v", err)
	}

	// A fresh process reads the durable state and the JSON contract.
	stdout, stderr, code := runOut(t, home, "skill", "show", "demo", "--json")
	if code != 0 {
		t.Fatalf("skill show --json: exit %d\n%s%s", code, stdout, stderr)
	}
	var sk struct {
		SyncStatus string `json:"sync_status"`
		LastSync   *struct {
			Action string `json:"action"`
			Result string `json:"result"`
		} `json:"last_sync"`
	}
	if err := json.Unmarshal([]byte(stdout), &sk); err != nil {
		t.Fatalf("skill show --json: %v\n%s", err, stdout)
	}
	if sk.SyncStatus != "store_changed" {
		t.Fatalf("durable sync status: %+v", sk)
	}
	if sk.LastSync == nil || sk.LastSync.Action != "rollback" || sk.LastSync.Result != "rolled_back" {
		t.Fatalf("durable last sync: %+v", sk.LastSync)
	}

	// The reverse rollback restores the accepted content in a fresh
	// process.
	out, code = run(t, home, "skill", "rollback", "demo")
	if code != 0 || !strings.Contains(out, "rolled_back") {
		t.Fatalf("reverse rollback: exit %d\n%s", code, out)
	}
	if data, err := os.ReadFile(filepath.Join(store, "demo", "notes.md")); err != nil || string(data) != "upstream\n" {
		t.Fatalf("reverse rollback content: %q, %v", data, err)
	}

	// diff renders the conflict surface as unified text when Store and
	// Source diverge again.
	if err := os.WriteFile(filepath.Join(root, "skills", "demo", "extra.md"), []byte("more upstream\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = run(t, home, "skill", "diff", "demo", "--path", "extra.md")
	if code != 0 || !strings.Contains(out, "+more upstream") {
		t.Fatalf("skill diff: exit %d\n%s", code, out)
	}
}

// TestCLISourceSyncBatchEndToEnd locks the batch synchronization journey
// across processes: one Source sync updates every bound Skill and exits 1
// with the complete result when any item is skipped.
func TestCLISourceSyncBatchEndToEnd(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "alpha")
	writeE2ESkill(t, root, "beta")
	if out, code := run(t, home, "source", "add", root, "--name", "batch-local"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "batch-local", "--all"); code != 0 {
		t.Fatalf("skill import: exit %d\n%s", code, out)
	}
	// One local edit forces a conflict for beta; alpha updates cleanly.
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", "notes.md"), []byte("alpha v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "beta", "notes.md"), []byte("beta v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "beta", "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, home, "source", "sync", "batch-local")
	if code != 1 {
		t.Fatalf("batch with a skip must exit 1: exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "Updated \"alpha\"") || !strings.Contains(out, "Skipped \"beta\"") ||
		!strings.Contains(out, "1 updated") || !strings.Contains(out, "1 skipped") {
		t.Fatalf("batch output: %s", out)
	}
	// beta's local edit survived byte for byte.
	if data, err := os.ReadFile(filepath.Join(store, "beta", "local.txt")); err != nil || string(data) != "local\n" {
		t.Fatalf("beta local edit: %q, %v", data, err)
	}
	// The --json stdout is exactly one JSON value even on the exit-1 path.
	out, _, code = runOut(t, home, "source", "sync", "batch-local", "--json")
	if code != 1 {
		t.Fatalf("batch --json exit: %d", code)
	}
	var batch struct {
		Items []struct {
			Slug   string `json:"slug"`
			Result string `json:"result"`
		} `json:"items"`
		Summary struct {
			Skipped int `json:"skipped"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &batch); err != nil {
		t.Fatalf("batch --json stdout: %v\n%s", err, out)
	}
	if len(batch.Items) != 2 || batch.Summary.Skipped != 1 {
		t.Fatalf("batch --json contract: %+v", batch)
	}
}
