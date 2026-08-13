package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/app"
)

// TestTargetsCLILifecycle drives the target vocabulary through the real
// command tree: custom and built-in registration, dedupe by physical path,
// assignments, and the desired set with explanations.
func TestTargetsCLILifecycle(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
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
	if out, err := runCmd(t, NewGroupCreateCmd(bm), "eng"); err != nil {
		t.Fatalf("group create: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewGroupAddSkillCmd(bm), "eng", "alpha", "beta"); err != nil {
		t.Fatalf("group add-skill: %v\n%s", err, out)
	}

	// custom registration with ~ expansion
	out, err := runCmd(t, NewTargetAddCmd(bm), "--path", "~/skills", "--name", "mine")
	if err != nil {
		t.Fatalf("target add: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Registered Target") || !strings.Contains(out, "-> "+filepath.Join(home, "skills")) {
		t.Fatalf("add output: %q", out)
	}
	// the same physical path returns the existing Target
	out, err = runCmd(t, NewTargetAddCmd(bm), "--path", filepath.Join(home, "skills"), "--name", "alias")
	if err != nil {
		t.Fatalf("target add same path: %v\n%s", err, out)
	}
	if !strings.Contains(out, "\"mine\"") {
		t.Fatalf("same-path add must return the existing Target: %q", out)
	}

	// built-in user target
	out, err = runCmd(t, NewTargetAddCmd(bm), "--adapter", "pi", "--scope", "user")
	if err != nil {
		t.Fatalf("target add pi: %v\n%s", err, out)
	}
	if !strings.Contains(out, "-> "+filepath.Join(home, ".pi", "agent", "skills")) {
		t.Fatalf("pi add output: %q", out)
	}

	// assignments
	if out, err = runCmd(t, NewTargetAssignCmd(bm), "mine", "--group", "eng"); err != nil {
		t.Fatalf("assign group: %v\n%s", err, out)
	}
	if out, err = runCmd(t, NewTargetAssignCmd(bm), "mine", "--skill", "alpha"); err != nil {
		t.Fatalf("assign skill: %v\n%s", err, out)
	}

	// show --json carries the expanded desired set with reasons
	out, err = runCmd(t, NewTargetShowCmd(bm), "mine", "--json")
	if err != nil {
		t.Fatalf("show --json: %v\n%s", err, out)
	}
	var view struct {
		ID            int64            `json:"id"`
		Name          string           `json:"name"`
		Path          string           `json:"path"`
		Adapter       string           `json:"adapter"`
		Scope         string           `json:"scope"`
		DirectSkills  []map[string]any `json:"direct_skills"`
		Groups        []map[string]any `json:"groups"`
		DesiredSkills []struct {
			Slug    string `json:"slug"`
			Reasons []struct {
				Kind      string `json:"kind"`
				GroupName string `json:"group_name"`
			} `json:"reasons"`
		} `json:"desired_skills"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("show --json: %v\n%s", err, out)
	}
	if view.Adapter != "custom" || view.Scope != "custom" || view.Path != filepath.Join(home, "skills") {
		t.Fatalf("view identity: %+v", view)
	}
	if len(view.DirectSkills) != 1 || len(view.Groups) != 1 {
		t.Fatalf("assignments: %+v / %+v", view.DirectSkills, view.Groups)
	}
	if len(view.DesiredSkills) != 2 {
		t.Fatalf("desired: %+v", view.DesiredSkills)
	}
	var alpha, beta bool
	for _, d := range view.DesiredSkills {
		if d.Slug == "alpha" && len(d.Reasons) == 2 {
			alpha = true
			var direct, via bool
			for _, r := range d.Reasons {
				if r.Kind == "skill" {
					direct = true
				}
				if r.Kind == "group" && r.GroupName == "eng" {
					via = true
				}
			}
			if !direct || !via {
				t.Fatalf("alpha reasons: %+v", d.Reasons)
			}
		}
		if d.Slug == "beta" && len(d.Reasons) == 1 && d.Reasons[0].Kind == "group" {
			beta = true
		}
	}
	if !alpha || !beta {
		t.Fatalf("desired set with reasons: %+v", view.DesiredSkills)
	}

	// unassign --group refreshes the desired set
	out, err = runCmd(t, NewTargetUnassignCmd(bm), "mine", "--group", "eng")
	if err != nil {
		t.Fatalf("unassign: %v\n%s", err, out)
	}
	out, err = runCmd(t, NewTargetShowCmd(bm), "mine", "--json")
	if err != nil {
		t.Fatalf("show after unassign: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.DesiredSkills) != 1 || view.DesiredSkills[0].Slug != "alpha" ||
		len(view.DesiredSkills[0].Reasons) != 1 {
		t.Fatalf("desired after unassign: %+v", view.DesiredSkills)
	}

	// list
	out, err = runCmd(t, NewTargetListCmd(bm))
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "mine") || !strings.Contains(out, "pi-user") || !strings.Contains(out, "2 Target(s)") {
		t.Fatalf("list output: %q", out)
	}
}

// TestTargetsCLIFlagContract pins the strict pre-state flag validation:
// exclusive forms and their requirements fail before any application call.
func TestTargetsCLIFlagContract(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	cases := [][]string{
		{},                                   // neither path nor adapter
		{"--path", "~/x", "--adapter", "pi"}, // mixed forms
		{"--adapter", "pi"},                  // missing scope
		{"--adapter", "pi", "--scope", "wat"},
		{"--adapter", "pi", "--scope", "project"},                        // missing project
		{"--adapter", "pi", "--scope", "user", "--project", t.TempDir()}, // project with user scope
	}
	for _, args := range cases {
		if _, err := runCmd(t, NewTargetAddCmd(bm), args...); err == nil || !isInvalidArgument(err) {
			t.Fatalf("args %v: got %v, want invalid_argument", args, err)
		}
	}
	// assign/unassign require exactly one subject flag
	for _, args := range [][]string{
		{"mine"},
		{"mine", "--skill", "a", "--group", "g"},
	} {
		if _, err := runCmd(t, NewTargetAssignCmd(bm), args...); err == nil || !isInvalidArgument(err) {
			t.Fatalf("assign args %v: got %v, want invalid_argument", args, err)
		}
	}
}

func isInvalidArgument(err error) bool {
	var ae *app.Error
	return errors.As(err, &ae) && ae.Code == app.CodeInvalidArgument
}
