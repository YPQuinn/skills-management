package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestGroupsCLILifecycle drives the group vocabulary through the real
// command tree: create, list, membership changes, and show with the
// operator-facing human output.
func TestGroupsCLILifecycle(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	writeCLISkill(t, root, "beta")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("skill import: %v\n%s", err, out)
	}

	out, err := runCmd(t, NewGroupCreateCmd(bm), "engineering")
	if err != nil {
		t.Fatalf("group create: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Created Group \"engineering\"") {
		t.Fatalf("create output: %q", out)
	}
	// duplicate name is a conflict
	if _, err := runCmd(t, NewGroupCreateCmd(bm), "engineering"); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate create: %v", err)
	}

	out, err = runCmd(t, NewGroupAddSkillCmd(bm), "engineering", "alpha", "beta")
	if err != nil {
		t.Fatalf("add-skill: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Added 2 Skill(s) to Group \"engineering\" (2 member(s))") {
		t.Fatalf("add-skill output: %q", out)
	}
	// idempotent re-add
	if out, err = runCmd(t, NewGroupAddSkillCmd(bm), "engineering", "alpha"); err != nil {
		t.Fatalf("add-skill again: %v\n%s", err, out)
	}

	out, err = runCmd(t, NewGroupListCmd(bm))
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "engineering\t2 Skill(s)") || !strings.Contains(out, "1 Group(s)") {
		t.Fatalf("list output: %q", out)
	}

	out, err = runCmd(t, NewGroupShowCmd(bm), "engineering")
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if !strings.Contains(out, `Members: "alpha" (alpha), "beta" (beta)`) {
		t.Fatalf("show members: %q", out)
	}

	out, err = runCmd(t, NewGroupRemoveSkillCmd(bm), "engineering", "beta")
	if err != nil {
		t.Fatalf("remove-skill: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Removed 1 Skill(s) from Group") {
		t.Fatalf("remove-skill output: %q", out)
	}

	// unknown Group and Skill arguments
	if _, err := runCmd(t, NewGroupShowCmd(bm), "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("show missing: %v", err)
	}
	if _, err := runCmd(t, NewGroupAddSkillCmd(bm), "engineering", "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("add-skill missing: %v", err)
	}
}

// TestGroupsCLIJSON pins the --json shapes for the group vocabulary.
func TestGroupsCLIJSON(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("skill import: %v\n%s", err, out)
	}
	out, err := runCmd(t, NewGroupCreateCmd(bm), "eng", "--json")
	if err != nil {
		t.Fatalf("create --json: %v\n%s", err, out)
	}
	m := rawCLIJSON(t, out)
	requireWireKeys(t, m, "id", "name", "created_at", "updated_at")

	out, err = runCmd(t, NewGroupAddSkillCmd(bm), "eng", "alpha", "--json")
	if err != nil {
		t.Fatalf("add-skill --json: %v\n%s", err, out)
	}
	m = rawCLIJSON(t, out)
	requireWireKeys(t, m, "id", "name", "members", "targets")
	var members []map[string]any
	if err := json.Unmarshal([]byte(out), &struct {
		Members *[]map[string]any `json:"members"`
	}{Members: &members}); err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0]["slug"] != "alpha" {
		t.Fatalf("members: %v", members)
	}

	out, err = runCmd(t, NewGroupListCmd(bm), "--json")
	if err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	m = rawCLIJSON(t, out)
	requireWireKeys(t, m, "items", "total")
}
