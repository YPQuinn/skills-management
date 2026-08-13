package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIGroupsTargetsLifecycle drives the Groups/Targets/Assignments
// journey through the real binary in separate processes: Group membership,
// custom and built-in Target registration with fixed physical-path
// identity, Assignments, and desired-set expansion with explanations.
func TestCLIGroupsTargetsLifecycle(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}

	root := t.TempDir()
	writeE2ESkill(t, root, "alpha")
	writeE2ESkill(t, root, "beta")
	if out, code := run(t, home, "source", "add", root, "--name", "e2e-local"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "e2e-local", "--all"); code != 0 {
		t.Fatalf("skill import: exit %d\n%s", code, out)
	}

	// Group membership
	if out, code := run(t, home, "group", "create", "engineering"); code != 0 {
		t.Fatalf("group create: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "group", "add-skill", "engineering", "alpha"); code != 0 {
		t.Fatalf("group add-skill: exit %d\n%s", code, out)
	}

	// custom Target registration
	container := filepath.Join(home, "skills")
	out, code := run(t, home, "target", "add", "--path", "~/skills", "--name", "mine")
	if code != 0 {
		t.Fatalf("target add: exit %d\n%s", code, out)
	}
	if !strings.Contains(out, container) {
		t.Fatalf("target add output must carry the resolved physical path:\n%s", out)
	}

	// the fixed physical path deduplicates across processes
	out, code = run(t, home, "target", "add", "--path", container, "--name", "alias")
	if code != 0 {
		t.Fatalf("target add same path: exit %d\n%s", code, out)
	}
	if !strings.Contains(out, `"mine"`) {
		t.Fatalf("same-path registration must return the existing Target:\n%s", out)
	}

	// built-in user Target
	out, code = run(t, home, "target", "add", "--adapter", "pi", "--scope", "user")
	if code != 0 {
		t.Fatalf("target add pi: exit %d\n%s", code, out)
	}
	if !strings.Contains(out, filepath.Join(home, ".pi", "agent", "skills")) {
		t.Fatalf("pi target path:\n%s", out)
	}

	// Assignment and desired-set expansion
	if out, code := run(t, home, "target", "assign", "mine", "--group", "engineering"); code != 0 {
		t.Fatalf("target assign: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "target", "show", "mine", "--json")
	if code != 0 {
		t.Fatalf("target show --json: exit %d\n%s", code, out)
	}
	var view struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Path    string `json:"path"`
		Scope   string `json:"scope"`
		Desired []struct {
			Slug    string `json:"slug"`
			Reasons []struct {
				Kind      string `json:"kind"`
				GroupName string `json:"group_name"`
			} `json:"reasons"`
		} `json:"desired_skills"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("target show --json is not one JSON value: %v\n%s", err, out)
	}
	if view.Path != container || view.Scope != "custom" {
		t.Fatalf("target identity: %+v", view)
	}
	if len(view.Desired) != 1 || view.Desired[0].Slug != "alpha" ||
		len(view.Desired[0].Reasons) != 1 || view.Desired[0].Reasons[0].Kind != "group" ||
		view.Desired[0].Reasons[0].GroupName != "engineering" {
		t.Fatalf("desired set: %+v", view.Desired)
	}

	// direct Skill Assignment expands and unassign shrinks the desired set
	if out, code := run(t, home, "target", "assign", "mine", "--skill", "beta"); code != 0 {
		t.Fatalf("assign skill: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "target", "show", "mine", "--json")
	if code != 0 {
		t.Fatalf("show after skill assign: exit %d\n%s", code, out)
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Desired) != 2 {
		t.Fatalf("desired after skill assign: %+v", view.Desired)
	}
	if out, code := run(t, home, "target", "unassign", "mine", "--group", "engineering"); code != 0 {
		t.Fatalf("unassign: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "target", "show", "mine", "--json")
	if code != 0 {
		t.Fatalf("show after unassign: exit %d\n%s", code, out)
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Desired) != 1 || view.Desired[0].Slug != "beta" ||
		len(view.Desired[0].Reasons) != 1 || view.Desired[0].Reasons[0].Kind != "skill" {
		t.Fatalf("desired after unassign: %+v", view.Desired)
	}

	// list carries both Targets; the durable state survives new processes
	out, code = run(t, home, "target", "list", "--json")
	if code != 0 {
		t.Fatalf("target list --json: exit %d\n%s", code, out)
	}
	var list struct {
		Items []struct {
			ID      int64  `json:"id"`
			Name    string `json:"name"`
			Path    string `json:"path"`
			Adapter string `json:"adapter"`
			Scope   string `json:"scope"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("target list --json: %v\n%s", err, out)
	}
	if list.Total != 2 {
		t.Fatalf("target list: %+v", list)
	}
}

// TestCLITargetFlagValidation pins the strict argument-validation exit
// codes for the target vocabulary (contract: 2 = validation).
func TestCLITargetFlagValidation(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	cases := [][]string{
		{"target", "add"},
		{"target", "add", "--path", "~/x", "--adapter", "pi"},
		{"target", "add", "--adapter", "pi"},
		{"target", "add", "--adapter", "pi", "--scope", "nope"},
		{"target", "add", "--adapter", "pi", "--scope", "project"},
		{"target", "assign", "mine"},
		{"target", "assign", "mine", "--skill", "a", "--group", "g"},
		{"target", "unassign", "mine"},
	}
	for _, args := range cases {
		if out, code := run(t, home, args...); code != 2 {
			t.Fatalf("%v: exit %d, want 2\n%s", args, code, out)
		}
	}
}
