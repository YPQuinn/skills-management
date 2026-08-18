package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/app"
)

// TestTargetDistributionCLI drives the distribution vocabulary through the
// real command tree: status refresh, dry-run planning, distribution with a
// real absolute symlink, adoption, and the blocked exit contract.
func TestTargetDistributionCLI(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	store := filepath.Join(t.TempDir(), "store")
	bm := initManager(t, store)
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("skill import: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewTargetAddCmd(bm), "--path", "~/skills", "--name", "mine"); err != nil {
		t.Fatalf("target add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewTargetAssignCmd(bm), "mine", "--skill", "alpha"); err != nil {
		t.Fatalf("target assign: %v\n%s", err, out)
	}

	// status refresh observes the desired Skill as missing
	out, err := runCmd(t, NewTargetStatusCmd(bm), "mine", "--refresh")
	if err != nil {
		t.Fatalf("target status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "alpha: present + missing") {
		t.Fatalf("status output: %q", out)
	}

	// the dry run predicts the create and mutates nothing
	out, err = runCmd(t, NewTargetDistributeCmd(bm), "mine", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "plan: succeeded") || !strings.Contains(out, "created \"alpha\"") {
		t.Fatalf("dry run output: %q", out)
	}
	link := filepath.Join(home, "skills", "alpha")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("dry run must not create the link")
	}

	// distribution creates the real absolute symlink
	out, err = runCmd(t, NewTargetDistributeCmd(bm), "mine")
	if err != nil {
		t.Fatalf("distribute: %v\n%s", err, out)
	}
	if !strings.Contains(out, "distribution: succeeded") {
		t.Fatalf("distribute output: %q", out)
	}
	raw, err := os.Readlink(link)
	if err != nil || !filepath.IsAbs(raw) {
		t.Fatalf("link: %q, %v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
		t.Fatalf("link does not resolve: %v", err)
	}

	// a conflict blocks with exit 1 while retaining the complete result
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runCmd(t, NewTargetDistributeCmd(bm), "mine")
	var pf partialFailure
	if err == nil || !errors.As(err, &pf) {
		t.Fatalf("blocked distribute must exit partially: %v\n%s", err, out)
	}
	if !strings.Contains(out, "blocked") || !strings.Contains(out, "blocked_conflict") {
		t.Fatalf("blocked output: %q", out)
	}
	if data, _ := os.ReadFile(link); string(data) != "mine" {
		t.Fatal("foreign content must be preserved")
	}

	// adopt after the user replaces the file with the correct symlink
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	storeRoot, err := filepath.EvalSymlinks(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(storeRoot, "alpha"), link); err != nil {
		t.Fatal(err)
	}
	out, err = runCmd(t, NewTargetAdoptCmd(bm), "mine", "alpha")
	if err != nil {
		t.Fatalf("adopt: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Adopted \"alpha\"") {
		t.Fatalf("adopt output: %q", out)
	}

	// after adoption the distribution is satisfied
	out, err = runCmd(t, NewTargetDistributeCmd(bm), "mine")
	if err != nil {
		t.Fatalf("post-adopt distribute: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no_op") {
		t.Fatalf("post-adopt output: %q", out)
	}

	// the JSON contracts expose the same shapes
	out, err = runCmd(t, NewTargetDistributeCmd(bm), "mine", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("json dry run: %v\n%s", err, out)
	}
	var res app.DistributionResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("json parse: %v\n%s", err, out)
	}
	if res.TargetID == 0 || len(res.Items) != 1 || res.Items[0].Result != "no_op" {
		t.Fatalf("json result: %+v", res)
	}
	out, err = runCmd(t, NewTargetStatusCmd(bm), "mine", "--json")
	if err != nil {
		t.Fatalf("json status: %v\n%s", err, out)
	}
	var st app.DistributionStatus
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("json status parse: %v\n%s", err, out)
	}
	if len(st.Items) != 1 || st.Items[0].Observed != "linked" {
		t.Fatalf("json status: %+v", st)
	}
}
