package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ECleanupLifecycles(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(home, "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}

	root := t.TempDir()
	writeE2ESkill(t, root, "demo")
	if out, code := run(t, home, "source", "add", root, "--name", "local"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "local", "--all"); code != 0 {
		t.Fatalf("import: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", "~/skills", "--name", "mine"); code != 0 {
		t.Fatalf("target add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "mine", "--skill", "demo"); code != 0 {
		t.Fatalf("assign: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "distribute", "mine"); code != 0 {
		t.Fatalf("distribute: exit %d\n%s", code, out)
	}
	link := filepath.Join(home, "skills", "demo")
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("managed link: %v", err)
	}

	if out, code := run(t, home, "skill", "delete", "demo", "--yes"); code == 0 {
		t.Fatalf("referenced delete must fail\n%s", out)
	}
	stdout, stderr, code := runOut(t, home, "skill", "delete", "demo", "--cleanup", "--yes", "--json")
	if code != 0 {
		t.Fatalf("skill delete: exit %d\n%s%s", code, stdout, stderr)
	}
	var res struct {
		Skill struct {
			Slug string `json:"slug"`
		} `json:"skill"`
		Links []struct {
			Result string `json:"result"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || res.Skill.Slug != "demo" {
		t.Fatalf("skill delete json: %v %s", err, stdout)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("managed link must be gone: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, "skills")); err != nil {
		t.Fatalf("container must remain: %v", err)
	}

	writeE2ESkill(t, root, "kept")
	if out, code := run(t, home, "source", "check", "local"); code != 0 {
		t.Fatalf("source check: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "local", "--path", "skills/kept"); code != 0 {
		t.Fatalf("import kept: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "detach", "kept", "--yes"); code != 0 {
		t.Fatalf("detach: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "source", "delete", "local", "--yes"); code != 0 {
		t.Fatalf("source delete: exit %d\n%s", code, out)
	}
	stdout, stderr, code = runOut(t, home, "skill", "show", "kept", "--json")
	if code != 0 {
		t.Fatalf("show kept: exit %d\n%s%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"sync_status": "unbound"`) {
		t.Fatalf("kept must stay unbound: %s", stdout)
	}
}
