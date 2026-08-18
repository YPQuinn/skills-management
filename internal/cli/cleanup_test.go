package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/app"
)

func TestCleanupCLISourceAndSkill(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if _, err := runCmd(t, NewSourceDeleteCmd(bm), "local", "--json"); err == nil {
		t.Fatal("json delete without --yes must fail")
	}
	if _, err := runCmd(t, NewSourceDeleteCmd(bm), "local", "--yes"); err == nil {
		t.Fatal("bound source without --detach-skills must fail")
	}
	out, err := runCmd(t, NewSourceDeleteCmd(bm), "local", "--detach-skills", "--yes", "--json")
	if err != nil {
		t.Fatalf("source delete: %v\n%s", err, out)
	}
	var srcRes app.SourceDeleteResult
	if jerr := json.Unmarshal([]byte(out), &srcRes); jerr != nil || len(srcRes.Detached) != 1 {
		t.Fatalf("source json: %v %s", jerr, out)
	}

	out, err = runCmd(t, NewSkillShowCmd(bm), "alpha", "--json")
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"sync_status": "unbound"`) {
		t.Fatalf("unbound: %s", out)
	}

	rootB := t.TempDir()
	writeCLISkill(t, rootB, "alpha")
	if out, err := runCmd(t, NewSourceAddCmd(bm), rootB, "--name", "other"); err != nil {
		t.Fatalf("source add other: %v\n%s", err, out)
	}
	out, err = runCmd(t, NewSkillRebindCmd(bm), "alpha", "--source", "other", "--path", "skills/alpha", "--yes", "--json")
	if err != nil {
		t.Fatalf("rebind: %v\n%s", err, out)
	}
	var rebind app.RebindResult
	if jerr := json.Unmarshal([]byte(out), &rebind); jerr != nil || !rebind.Identical {
		t.Fatalf("rebind json: %v %s", jerr, out)
	}

	out, err = runCmd(t, NewSkillDeleteCmd(bm), "alpha", "--yes", "--json")
	if err != nil {
		t.Fatalf("skill delete: %v\n%s", err, out)
	}
	var del app.SkillDeleteResult
	if jerr := json.Unmarshal([]byte(out), &del); jerr != nil || del.Skill.Slug != "alpha" {
		t.Fatalf("skill delete json: %v %s", jerr, out)
	}
}

func TestCleanupCLITargetAndGroup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewGroupCreateCmd(bm), "eng"); err != nil {
		t.Fatalf("group: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewGroupAddSkillCmd(bm), "eng", "alpha"); err != nil {
		t.Fatalf("add-skill: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewTargetAddCmd(bm), "--path", "~/skills", "--name", "mine"); err != nil {
		t.Fatalf("target add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewTargetAssignCmd(bm), "mine", "--group", "eng"); err != nil {
		t.Fatalf("assign: %v\n%s", err, out)
	}
	if _, err := runCmd(t, NewGroupDeleteCmd(bm), "eng", "--yes"); err == nil {
		t.Fatal("assigned group must be blocked")
	}
	if out, err := runCmd(t, NewGroupDeleteCmd(bm), "eng", "--unassign", "--yes"); err != nil {
		t.Fatalf("group delete: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewTargetDeleteCmd(bm), "mine", "--yes", "--json"); err != nil {
		t.Fatalf("target delete: %v\n%s", err, out)
	} else {
		var res app.TargetDeleteResult
		if jerr := json.Unmarshal([]byte(out), &res); jerr != nil || res.Target.Name != "mine" {
			t.Fatalf("target json: %v %s", jerr, out)
		}
	}
}
