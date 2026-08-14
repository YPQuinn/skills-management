package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/sync"
)

// TestSyncCLILifecycle drives the noun-first synchronization vocabulary
// through the real command tree: check, sync, keep-store, accept-source,
// rollback, and the Source batch sync, with their human outputs.
func TestSyncCLILifecycle(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "demo")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("skill import: %v\n%s", err, out)
	}

	// check shows the evaluated in_sync state.
	out, err := runCmd(t, NewSkillCheckSyncCmd(bm), "demo")
	if err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"demo": in_sync`) {
		t.Fatalf("check output: %q", out)
	}

	// sync is a no-op while in sync.
	out, err = runCmd(t, NewSkillSyncCmd(bm), "demo")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no_op") {
		t.Fatalf("sync output: %q", out)
	}

	// Modify the Source; the batch sync updates.
	notes := filepath.Join(root, "skills", "demo", "notes.md")
	if err := os.WriteFile(notes, []byte("upstream\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runCmd(t, NewSourceSyncCmd(bm), "local")
	if err != nil {
		t.Fatalf("source sync: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Updated \"demo\"") || !strings.Contains(out, "1 updated") {
		t.Fatalf("source sync output: %q", out)
	}

	// rollback restores and reports store_changed.
	out, err = runCmd(t, NewSkillRollbackCmd(bm), "demo")
	if err != nil {
		t.Fatalf("rollback: %v\n%s", err, out)
	}
	if !strings.Contains(out, "rolled_back") || !strings.Contains(out, "store_changed") {
		t.Fatalf("rollback output: %q", out)
	}

	// diff prints the three comparisons and unified text while the rolled-
	// back Store differs from the accepted Baseline and Source.
	out, err = runCmd(t, NewSkillDiffCmd(bm), "demo", "--path", "notes.md")
	if err != nil {
		t.Fatalf("diff: %v\n%s", err, out)
	}
	if !strings.Contains(out, "baseline -> store") || !strings.Contains(out, "-upstream") {
		t.Fatalf("diff output: %q", out)
	}

	// reverse rollback.
	out, err = runCmd(t, NewSkillRollbackCmd(bm), "demo")
	if err != nil {
		t.Fatalf("reverse rollback: %v\n%s", err, out)
	}

	// keep-store and accept-source complete as no-ops on the converged
	// state.
	out, err = runCmd(t, NewSkillKeepStoreCmd(bm), "demo")
	if err != nil || !strings.Contains(out, "no_op") {
		t.Fatalf("keep-store: %v\n%s", err, out)
	}
	out, err = runCmd(t, NewSkillAcceptSourceCmd(bm), "demo")
	if err != nil || !strings.Contains(out, "no_op") {
		t.Fatalf("accept-source: %v\n%s", err, out)
	}
}

// TestSyncCLIJSONContract locks the --json shapes: exactly one JSON value
// for outcomes, the skill view for check, and the batch result for source
// sync.
func TestSyncCLIJSONContract(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "demo")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("skill import: %v\n%s", err, out)
	}
	out, err := runCmd(t, NewSkillSyncCmd(bm), "demo", "--json")
	if err != nil {
		t.Fatalf("sync --json: %v\n%s", err, out)
	}
	var item syncItemJSONView
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("sync --json value: %v\n%s", err, out)
	}
	if item.Result != "no_op" || item.Action != "sync" || item.SkillID == 0 {
		t.Fatalf("sync --json item: %+v", item)
	}

	out, err = runCmd(t, NewSkillCheckSyncCmd(bm), "demo", "--json")
	if err != nil {
		t.Fatalf("check --json: %v\n%s", err, out)
	}
	var sk struct {
		Slug       string `json:"slug"`
		SyncStatus string `json:"sync_status"`
	}
	if err := json.Unmarshal([]byte(out), &sk); err != nil {
		t.Fatalf("check --json value: %v\n%s", err, out)
	}
	if sk.Slug != "demo" || sk.SyncStatus != "in_sync" {
		t.Fatalf("check --json skill: %+v", sk)
	}

	out, err = runCmd(t, NewSourceSyncCmd(bm), "local", "--json")
	if err != nil {
		t.Fatalf("source sync --json: %v\n%s", err, out)
	}
	var batch syncBatchJSONView
	if err := json.Unmarshal([]byte(out), &batch); err != nil {
		t.Fatalf("source sync --json value: %v\n%s", err, out)
	}
	if batch.Summary.Total != 1 || batch.Summary.NoOp != 1 {
		t.Fatalf("source sync --json batch: %+v", batch)
	}
}

// TestSyncCLIBlockedExitsOne locks decision 09: a skipped or blocked use
// case prints its complete result and exits 1, never a second JSON value.
func TestSyncCLIBlockedExitsOne(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store")
	bm := initManager(t, storePath)
	root := t.TempDir()
	writeCLISkill(t, root, "demo")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("skill import: %v\n%s", err, out)
	}
	// A local Store edit plus a Source change is a conflict: sync skips
	// and exits 1 while retaining the complete result.
	if err := os.WriteFile(filepath.Join(root, "skills", "demo", "notes.md"), []byte("upstream\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	storeDir := filepath.Join(storePath, "demo")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, silentCmd(NewSkillSyncCmd(bm)), "demo", "--json")
	if err == nil {
		t.Fatalf("blocked sync must fail: %q", out)
	}
	var pf partialFailure
	if !errors.As(err, &pf) {
		t.Fatalf("blocked sync must be a partial failure: %T", err)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("blocked sync exit code: %d", ExitCode(err))
	}
	// The stdout carries exactly one JSON value: the complete result.
	var item syncItemJSONView
	if jerr := json.Unmarshal([]byte(out), &item); jerr != nil {
		t.Fatalf("blocked sync stdout: %v\n%q", jerr, out)
	}
	if item.Result != "skipped" || item.Status != "conflict" {
		t.Fatalf("blocked sync item: %+v", item)
	}
	// The batch reports the same exit-1 contract.
	out, err = runCmd(t, silentCmd(NewSourceSyncCmd(bm)), "local", "--json")
	if err == nil {
		t.Fatalf("blocked batch must fail: %q", out)
	}
	var batch syncBatchJSONView
	if jerr := json.Unmarshal([]byte(out), &batch); jerr != nil {
		t.Fatalf("blocked batch stdout: %v\n%q", jerr, out)
	}
	if batch.Summary.Skipped != 1 || batch.Items[0].Result != "skipped" {
		t.Fatalf("blocked batch: %+v", batch)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("blocked batch exit code: %d", ExitCode(err))
	}
}

// silentCmd silences Cobra's own error and usage printing so a test can
// assert the exact stdout of a failing command, mirroring the production
// root command whose SilenceErrors/SilenceUsage are set.
func silentCmd(cmd *cobra.Command) *cobra.Command {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd
}

// TestSyncCLIArgumentErrors locks exit code 2 for command-shape failures.
func TestSyncCLIArgumentErrors(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	if _, err := runCmd(t, NewSkillSyncCmd(bm)); err == nil || ExitCode(err) != 2 {
		t.Fatalf("sync without argument: %v", err)
	}
	if _, err := runCmd(t, NewSkillDiffCmd(bm), "a", "b"); err == nil || ExitCode(err) != 2 {
		t.Fatalf("diff with two arguments: %v", err)
	}
	// Rollback without a snapshot is a blocked use case: exit 1.
	root := t.TempDir()
	writeCLISkill(t, root, "demo")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("skill import: %v\n%s", err, out)
	}
	if _, err := runCmd(t, NewSkillRollbackCmd(bm), "demo"); err == nil || ExitCode(err) != 1 {
		t.Fatalf("rollback without snapshot: %v (exit %d)", err, ExitCode(err))
	}
}

// TestSkillShowReportsSyncState locks the extended show output.
func TestSkillShowReportsSyncState(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "demo")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local", "--all"); err != nil {
		t.Fatalf("skill import: %v\n%s", err, out)
	}
	if out, err := runCmd(t, NewSkillCheckSyncCmd(bm), "demo"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	out, err := runCmd(t, NewSkillShowCmd(bm), "demo")
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Sync status: in_sync") {
		t.Fatalf("show output: %q", out)
	}
	// The status vocabulary lives in the app contract.
	if sync.StatusInSync != "in_sync" || app.CodeConflict != "conflict" {
		t.Fatal("stable vocabulary drifted")
	}
}
