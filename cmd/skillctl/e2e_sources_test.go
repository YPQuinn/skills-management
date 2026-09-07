package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const e2eSkill = "---\nname: %s\ndescription: %s\n---\n# Body\n"

func writeE2ESkill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.ReplaceAll(strings.ReplaceAll(e2eSkill, "%s", name), "%s", "desc")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCLISourcesLocalLifecycle(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}

	// uninitialized-installation behavior in a fresh HOME
	fresh := t.TempDir()
	out, code := run(t, fresh, "source", "list")
	if code != 1 || !strings.Contains(out, "not initialized") {
		t.Fatalf("source list before init: exit %d\n%s", code, out)
	}

	root := t.TempDir()
	writeE2ESkill(t, root, "alpha")
	writeE2ESkill(t, root, "beta")

	// registration scans but never imports into the Store
	out, code = run(t, home, "source", "add", root, "--name", "e2e-local")
	if code != 0 || !strings.Contains(out, "Registered Source \"e2e-local\"") || !strings.Contains(out, "2 Skill(s)") {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	entries, err := os.ReadDir(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("registration must not import: Store contains %v", entries)
	}

	// list and parseable JSON output
	out, code = run(t, home, "source", "list")
	if code != 0 || !strings.Contains(out, "e2e-local") {
		t.Fatalf("source list: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "source", "list", "--json")
	if code != 0 {
		t.Fatalf("source list --json: exit %d\n%s", code, out)
	}
	var envelope struct {
		Items []struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Kind       string `json:"kind"`
			EntryCount int    `json:"entry_count"`
			Available  bool   `json:"available"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("list --json is not one valid JSON value: %v\n%s", err, out)
	}
	if envelope.Total != 1 || envelope.Items[0].Name != "e2e-local" || envelope.Items[0].Kind != "local" || envelope.Items[0].EntryCount != 2 || !envelope.Items[0].Available {
		t.Fatalf("list --json content: %+v", envelope)
	}
	id := envelope.Items[0].ID

	// show by name and by id agree
	out, code = run(t, home, "source", "show", "e2e-local")
	for _, want := range []string{"Source 1: e2e-local", "skills/alpha", "skills/beta", "Check result: ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q:\n%s", want, out)
		}
	}

	// check reflects a new upstream Skill
	writeE2ESkill(t, root, "gamma")
	out, code = run(t, home, "source", "check", fmt.Sprint(id), "--json")
	if code != 0 {
		t.Fatalf("source check: exit %d\n%s", code, out)
	}
	var checked struct {
		Inventory []struct {
			Name string `json:"name"`
		} `json:"inventory"`
	}
	if err := json.Unmarshal([]byte(out), &checked); err != nil || len(checked.Inventory) != 3 {
		t.Fatalf("check --json: %+v, %v\n%s", checked, err, out)
	}

	// the Source becomes unavailable: check succeeds, inventory is stale
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	out, code = run(t, home, "source", "check", "e2e-local")
	if code != 0 || !strings.Contains(out, "unavailable") {
		t.Fatalf("check unavailable: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "source", "show", "e2e-local")
	if code != 0 || !strings.Contains(out, "stale") || !strings.Contains(out, "skills/gamma") {
		t.Fatalf("stale show: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "source", "list", "--json")
	if code != 0 {
		t.Fatal("list --json after failure")
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil || envelope.Items[0].Available || !envelope.Items[0].Available == false {
		t.Fatalf("unavailable json list: %+v, %v", envelope, err)
	}

	// durable state across processes: a fresh binary process reads SQLite
	out, code = run(t, home, "source", "list")
	if code != 0 || !strings.Contains(out, "e2e-local") {
		t.Fatalf("list in a fresh process: exit %d\n%s", code, out)
	}
}

func TestCLISourcesGitLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	home := t.TempDir()
	if out, code := run(t, home, "init"); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}

	work := t.TempDir()
	g := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	g("init", "-b", "main")
	g("config", "user.email", "test@example.com")
	g("config", "user.name", "Test")
	writeE2ESkill(t, work, "repo-skill")
	g("add", ".")
	g("commit", "-m", "one")
	bare := filepath.Join(t.TempDir(), "remote.git")
	cmd := exec.Command("git", "clone", "--bare", work, bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone --bare: %v\n%s", err, out)
	}

	// GitHub-style shorthand is normalized; here we use a full file URL
	out, code := run(t, home, "source", "add", "file://"+bare, "--name", "e2e-git")
	if code != 0 || !strings.Contains(out, "1 Skill(s)") {
		t.Fatalf("source add git: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "source", "show", "e2e-git", "--json")
	if code != 0 {
		t.Fatalf("show git --json: exit %d\n%s", code, out)
	}
	var shown struct {
		Kind                string `json:"kind"`
		LastCommit          string `json:"last_commit"`
		LastCheckResult     string `json:"last_check_result"`
		LastInventoryDigest string `json:"last_inventory_digest"`
		Inventory           []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"inventory"`
	}
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.Kind != "git" || len(shown.LastCommit) != 40 || len(shown.Inventory) != 1 || shown.Inventory[0].Name != "repo-skill" {
		t.Fatalf("git show: %+v", shown)
	}
	if shown.LastCheckResult != "ok" || shown.LastInventoryDigest == "" {
		t.Fatalf("git show metadata: %+v", shown)
	}
	if shown.Inventory[0].Digest != "" {
		t.Fatalf("git add is listing-only and must omit complete-tree digests: %+v", shown.Inventory[0])
	}

	// upstream advances; a fresh check resolves the new commit
	writeE2ESkill(t, work, "second-skill")
	g("add", ".")
	g("commit", "-m", "two")
	if out, err := exec.Command("git", "-C", work, "push", bare, "main").CombinedOutput(); err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	out, code = run(t, home, "source", "check", "e2e-git", "--json")
	if code != 0 {
		t.Fatalf("git check: exit %d\n%s", code, out)
	}
	var checked struct {
		LastCommit string `json:"last_commit"`
		Inventory  []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"inventory"`
	}
	if err := json.Unmarshal([]byte(out), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.LastCommit == shown.LastCommit || len(checked.Inventory) != 2 {
		t.Fatalf("git check did not observe the update: %+v (was %+v)", checked, shown)
	}
	for _, e := range checked.Inventory {
		if e.Digest == "" {
			t.Fatalf("git check must fill complete-tree digests: %+v", checked.Inventory)
		}
	}

	// an unreachable repository is refused at registration
	out, code = run(t, home, "source", "add", "file://"+filepath.Join(t.TempDir(), "missing.git"))
	if code != 1 || !strings.Contains(out, "not reachable") {
		t.Fatalf("add unreachable git: exit %d\n%s", code, out)
	}
}
