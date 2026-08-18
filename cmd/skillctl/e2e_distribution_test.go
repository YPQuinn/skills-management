package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type e2eDistributionResult struct {
	TargetID int64  `json:"target_id"`
	DryRun   bool   `json:"dry_run"`
	Outcome  string `json:"outcome"`
	Items    []struct {
		SkillID  int64  `json:"skill_id"`
		Slug     string `json:"slug"`
		Desired  string `json:"desired"`
		Observed string `json:"observed"`
		Action   string `json:"action"`
		Result   string `json:"result"`
	} `json:"items"`
	Summary struct {
		Total           int `json:"total"`
		Created         int `json:"created"`
		Removed         int `json:"removed"`
		BlockedConflict int `json:"blocked_conflict"`
		OwnershipLost   int `json:"ownership_lost"`
	} `json:"summary"`
}

type e2eDistributionStatus struct {
	State string `json:"state"`
	Items []struct {
		Slug      string `json:"slug"`
		Desired   string `json:"desired"`
		Observed  string `json:"observed"`
		Adoptable bool   `json:"adoptable"`
		Managed   bool   `json:"managed"`
	} `json:"items"`
}

// e2eDistributionCLI runs the distribution journey of separate processes
// (decision 09): assign, refresh, dry-run, distribute with a real absolute
// symlink, unassign with managed-only removal, and durable state across
// processes.
func e2eDistributionCLI(t *testing.T, home, store string) {
	root := t.TempDir()
	writeE2ESkill(t, root, "demo")
	if out, code := run(t, home, "source", "add", root, "--name", "dist-local"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "dist-local", "--all"); code != 0 {
		t.Fatalf("skill import: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", "~/skills", "--name", "dist"); code != 0 {
		t.Fatalf("target add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "dist", "--skill", "demo"); code != 0 {
		t.Fatalf("target assign: exit %d\n%s", code, out)
	}

	// fresh status observes the desired Skill as missing
	stdout, stderr, code := runOut(t, home, "target", "status", "dist", "--refresh", "--json")
	if code != 0 {
		t.Fatalf("status: exit %d\n%s%s", code, stdout, stderr)
	}
	var st e2eDistributionStatus
	if err := json.Unmarshal([]byte(stdout), &st); err != nil {
		t.Fatalf("status json: %v\n%s", err, stdout)
	}
	if len(st.Items) != 1 || st.Items[0].Observed != "missing" {
		t.Fatalf("status: %+v", st)
	}

	// dry run predicts the create and mutates nothing
	stdout, stderr, code = runOut(t, home, "target", "distribute", "dist", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("dry run: exit %d\n%s%s", code, stdout, stderr)
	}
	var dry e2eDistributionResult
	if err := json.Unmarshal([]byte(stdout), &dry); err != nil {
		t.Fatalf("dry run json: %v\n%s", err, stdout)
	}
	if dry.Outcome != "succeeded" || dry.Items[0].Result != "created" {
		t.Fatalf("dry run: %+v", dry)
	}
	link := filepath.Join(home, "skills", "demo")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("dry run must not create the link")
	}

	// distribution creates the real absolute symlink
	stdout, stderr, code = runOut(t, home, "target", "distribute", "dist", "--json")
	if code != 0 {
		t.Fatalf("distribute: exit %d\n%s%s", code, stdout, stderr)
	}
	var res e2eDistributionResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("distribute json: %v\n%s", err, stdout)
	}
	if res.Outcome != "succeeded" || res.Summary.Created != 1 {
		t.Fatalf("distribute: %+v", res)
	}
	raw, err := os.Readlink(link)
	if err != nil || !filepath.IsAbs(raw) {
		t.Fatalf("link: %q, %v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
		t.Fatalf("link resolution: %v", err)
	}

	// an unmanaged same-name file produces a conflict and stays unchanged
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runOut(t, home, "target", "distribute", "dist", "--json")
	if code != 1 {
		t.Fatalf("blocked distribute: exit %d\n%s%s", code, stdout, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("blocked json: %v\n%s", err, stdout)
	}
	if res.Outcome != "blocked" || res.Items[0].Result != "blocked_conflict" {
		t.Fatalf("blocked: %+v", res)
	}
	if data, err := os.ReadFile(link); err != nil || string(data) != "mine" {
		t.Fatalf("foreign content: %q, %v", data, err)
	}

	// an unmanaged symlink to the correct Store Skill is not adopted
	// implicitly, but explicit adoption establishes ownership
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(store, "demo"), link); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runOut(t, home, "target", "distribute", "dist", "--json")
	if code != 1 {
		t.Fatalf("unmanaged symlink distribute: exit %d\n%s%s", code, stdout, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Result != "blocked_conflict" {
		t.Fatalf("no implicit adoption: %+v", res)
	}
	if out, code := run(t, home, "target", "adopt", "dist", "demo"); code != 0 || !strings.Contains(out, "Adopted") {
		t.Fatalf("adopt: exit %d\n%s", code, out)
	}

	// remove the Assignment and redistribute: only the Managed Link goes
	if out, code := run(t, home, "target", "unassign", "dist", "--skill", "demo"); code != 0 {
		t.Fatalf("unassign: exit %d\n%s", code, out)
	}
	stdout, stderr, code = runOut(t, home, "target", "distribute", "dist", "--json")
	if code != 0 {
		t.Fatalf("removal distribute: exit %d\n%s%s", code, stdout, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Result != "removed" {
		t.Fatalf("removal: %+v", res)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("only the Managed Link may be removed")
	}
}

// TestCLIDistributionEndToEnd runs the distribution journey in isolated
// separate processes and reopens the state to prove durability.
func TestCLIDistributionEndToEnd(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	e2eDistributionCLI(t, home, store)

	// a new process sees the durable state
	out, code := run(t, home, "target", "status", "dist")
	if code != 0 || !strings.Contains(out, "dist") {
		t.Fatalf("reopen status: exit %d\n%s", code, out)
	}
}
