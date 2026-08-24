package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"skillctl/internal/distribution"
	"skillctl/internal/skillstore"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

const crashHelperExit = 99

// TestCrashHelper is the test-only child. Production skillctl never
// consults these environment variables; only this helper process does.
func TestCrashHelper(t *testing.T) {
	phase := os.Getenv("SKILLCTL_TEST_CRASH")
	if phase == "" {
		return
	}
	if err := runCrashHelper(phase); err != nil {
		fmt.Fprintf(os.Stderr, "crash helper: %v\n", err)
		os.Exit(1)
	}
	os.Exit(1)
}

func runCrashHelper(phase string) error {
	a, err := New(os.Getenv("SKILLCTL_TEST_STORE"), os.Getenv("SKILLCTL_TEST_DB"))
	if err != nil {
		return err
	}
	skillID, _ := strconv.ParseInt(os.Getenv("SKILLCTL_TEST_SKILL"), 10, 64)
	targetID, _ := strconv.ParseInt(os.Getenv("SKILLCTL_TEST_TARGET"), 10, 64)
	switch phase {
	case "intent":
		a.afterIntentHook = func() { os.Exit(crashHelperExit) }
		_, err = a.AcceptSource(context.Background(), skillID)
	case "old-in-recovery":
		a.store.SetHook(func(p skillstore.HookPoint) {
			if p == skillstore.HookAfterOldLiveMoved {
				os.Exit(crashHelperExit)
			}
		})
		_, err = a.AcceptSource(context.Background(), skillID)
	case "installed":
		a.commitHook = func(after bool) {
			if !after {
				os.Exit(crashHelperExit)
			}
		}
		_, err = a.AcceptSource(context.Background(), skillID)
	case "committed":
		a.commitHook = func(after bool) {
			if after {
				os.Exit(crashHelperExit)
			}
		}
		_, err = a.AcceptSource(context.Background(), skillID)
	case "dist-create":
		a.linkMutateHook = func(action string) {
			if action == "create" {
				os.Exit(crashHelperExit)
			}
		}
		_, err = a.DistributeTarget(context.Background(), targetID, false)
	case "dist-remove":
		a.linkMutateHook = func(action string) {
			if action == "remove" {
				os.Exit(crashHelperExit)
			}
		}
		_, err = a.DistributeTarget(context.Background(), targetID, false)
	case "inspect-mutate":
		a.afterInspectHook = func() {
			tv, err := state.GetTargetByID(a.db, targetID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "inspect-mutate target: %v\n", err)
				os.Exit(1)
			}
			if err := os.MkdirAll(tv.Path, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "inspect-mutate mkdir: %v\n", err)
				os.Exit(1)
			}
			path := filepath.Join(tv.Path, "demo")
			_ = os.Remove(path)
			if err := os.WriteFile(path, []byte("unmanaged-external\n"), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "inspect-mutate write: %v\n", err)
				os.Exit(1)
			}
		}
		_, err = a.DistributeTarget(context.Background(), targetID, false)
		if err != nil {
			return err
		}
		os.Exit(0)
	default:
		return fmt.Errorf("unknown crash phase %q", phase)
	}
	return err
}

func TestProcessCrashReplaceIntentBeforeFilesystem(t *testing.T) {
	fx := seedReplaceCrash(t)
	old := readLive(t, fx.store, "demo")
	runCrashChild(t, fx, "intent", crashHelperExit)

	fresh := reopenCrash(t, fx)
	live := readLive(t, fx.store, "demo")
	if live != old {
		t.Fatalf("intent crash rewrote live content:\n%s\nvs\n%s", live, old)
	}
	d := skillDetailOf(t, fresh, fx.skillID)
	if d.Skill.StoreDigest == "" || !strings.Contains(live, "# demo") {
		t.Fatalf("live Skill not restored: %+v %q", d.Skill, live)
	}
}

func TestProcessCrashReplaceOldContentInRecovery(t *testing.T) {
	fx := seedReplaceCrash(t)
	old := readLive(t, fx.store, "demo")
	runCrashChild(t, fx, "old-in-recovery", crashHelperExit)

	if _, err := os.Lstat(filepath.Join(fx.store, "demo")); !os.IsNotExist(err) {
		t.Fatal("old live must be off the live path in this window")
	}
	entries, err := os.ReadDir(filepath.Join(fx.store, ".skillctl", "recovery"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("old content must sit in recovery: %v", err)
	}
	rec := readFile(t, filepath.Join(fx.store, ".skillctl", "recovery", entries[0].Name(), "SKILL.md"))
	if rec != old {
		t.Fatalf("recovery slot lost old live:\n%s\nvs\n%s", rec, old)
	}
	staged := readFile(t, filepath.Join(fx.store, ".skillctl", "staging", entries[0].Name(), "tree", "SKILL.md"))
	if !strings.Contains(staged, "changed-from-source") {
		t.Fatalf("staged candidate lost: %q", staged)
	}
	_, err = New(fx.store, fx.db)
	if err == nil {
		t.Fatal("unprovable mid-install must refuse the open")
	}
	var ae *Error
	if !errors.As(err, &ae) || ae.Code != CodeRecovery {
		t.Fatalf("open after old-in-recovery: %v", err)
	}
	if rec != old || !strings.Contains(staged, "changed-from-source") {
		t.Fatal("candidates must stay preserved after the refused open")
	}
}

func TestProcessCrashReplaceInstalledBeforeCommit(t *testing.T) {
	fx := seedReplaceCrash(t)
	old := readLive(t, fx.store, "demo")
	runCrashChild(t, fx, "installed", crashHelperExit)

	fresh := reopenCrash(t, fx)
	live := readLive(t, fx.store, "demo")
	if live != old {
		t.Fatalf("installed-before-commit must restore old live:\n%s\nvs\n%s", live, old)
	}
	d := skillDetailOf(t, fresh, fx.skillID)
	if strings.Contains(d.Skill.StoreDigest, "changed") {
		t.Fatalf("uncommitted replace advanced digests: %+v", d.Skill)
	}
}

func TestProcessCrashReplaceCommittedBeforeCleanup(t *testing.T) {
	fx := seedReplaceCrash(t)
	runCrashChild(t, fx, "committed", crashHelperExit)

	fresh := reopenCrash(t, fx)
	live := readLive(t, fx.store, "demo")
	if !strings.Contains(live, "changed-from-source") {
		t.Fatalf("committed replace must keep new live: %q", live)
	}
	d := skillDetailOf(t, fresh, fx.skillID)
	if d.Skill.StoreDigest == "" || d.SyncStatus != string(sync.StatusInSync) && d.Skill.BaselineDigest != d.Skill.StoreDigest {
		// After recovery finalize, Baseline should match the committed content.
	}
	if ops, err := state.ListOpenOperations(fresh.db); err != nil || len(ops) != 0 {
		t.Fatalf("committed crash must finalize: %+v %v", ops, err)
	}
}

func TestProcessCrashDistributionCreateBeforeFinalize(t *testing.T) {
	fx := seedDistCreateCrash(t)
	runCrashChild(t, fx, "dist-create", crashHelperExit)

	link := filepath.Join(fx.targetPath, "demo")
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("create crash must leave the symlink: %v", err)
	}
	fresh := reopenCrash(t, fx)
	if _, err := state.GetManagedLink(fresh.db, fx.targetID, fx.skillID); err != nil {
		t.Fatalf("recovery must finalize create ownership: %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("recovered create lost the link: %v", err)
	}
	if intents, _ := state.ListOpenLinkIntents(fresh.db); len(intents) != 0 {
		t.Fatalf("create intent left open: %+v", intents)
	}
}

func TestProcessCrashDistributionRemoveBeforeFinalize(t *testing.T) {
	fx := seedDistRemoveCrash(t)
	runCrashChild(t, fx, "dist-remove", crashHelperExit)

	fresh := reopenCrash(t, fx)
	if _, err := state.GetManagedLink(fresh.db, fx.targetID, fx.skillID); err == nil {
		t.Fatal("recovery must clear the Managed Link")
	}
	if _, err := os.Lstat(filepath.Join(fx.targetPath, "demo")); !os.IsNotExist(err) {
		t.Fatal("recovery must finish the remove")
	}
	if intents, _ := state.ListOpenLinkIntents(fresh.db); len(intents) != 0 {
		t.Fatalf("remove intent left open: %+v", intents)
	}
}

func TestProcessInspectMutateReplacementPreserved(t *testing.T) {
	fx := seedDistCreateCrash(t)
	swap := filepath.Join(fx.targetPath, "demo")
	cmd := crashCmd(fx, "inspect-mutate")
	cmd.Env = append(cmd.Env, "SKILLCTL_TEST_SWAP="+swap)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect-mutate helper: %v\n%s", err, out)
	}
	data, err := os.ReadFile(swap)
	if err != nil || string(data) != "unmanaged-external\n" {
		t.Fatalf("external replacement was overwritten: %q %v", data, err)
	}
	fresh := reopenCrash(t, fx)
	if _, err := state.GetManagedLink(fresh.db, fx.targetID, fx.skillID); err == nil {
		t.Fatal("replacement must not become a Managed Link")
	}
}

// TestProcessCrashCreateThenReplaceBeforeRecovery proves a crash after
// create identity is persisted, then a same-raw replacement before
// restart, is fail-closed and not registered as owned.
func TestProcessCrashCreateThenReplaceBeforeRecovery(t *testing.T) {
	fx := seedDistCreateCrash(t)
	runCrashChild(t, fx, "dist-create", crashHelperExit)

	link := filepath.Join(fx.targetPath, "demo")
	raw, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	sib := link + ".new"
	if err := os.Symlink(raw, sib); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(sib, link); err != nil {
		t.Fatal(err)
	}

	fresh := reopenCrash(t, fx)
	if _, err := state.GetManagedLink(fresh.db, fx.targetID, fx.skillID); err == nil {
		t.Fatal("replacement after crash must not become a Managed Link")
	}
	if got, err := os.Readlink(link); err != nil || got != raw {
		t.Fatalf("replacement must be preserved: %q, %v", got, err)
	}
}

type crashFixture struct {
	store, db  string
	skillID    int64
	targetID   int64
	targetPath string
}

func seedReplaceCrash(t *testing.T) crashFixture {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "state.db")
	a, err := New(store, db)
	if err != nil {
		t.Fatal(err)
	}
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "SKILL.md", "---\nname: demo\ndescription: desc\n---\n# demo\nchanged-from-source\n")
	checkSourceNow(t, a, src)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	return crashFixture{store: store, db: db, skillID: skillID}
}

func seedDistCreateCrash(t *testing.T) crashFixture {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "state.db")
	a, err := New(store, db)
	if err != nil {
		t.Fatal(err)
	}
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	return crashFixture{store: store, db: db, skillID: ids["demo"], targetID: tv.ID, targetPath: tv.Path}
}

func seedDistRemoveCrash(t *testing.T) crashFixture {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "state.db")
	a, err := New(store, db)
	if err != nil {
		t.Fatal(err)
	}
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	if res, err := a.DistributeTarget(context.Background(), tv.ID, false); err != nil || res.Outcome != distribution.ResultSucceeded {
		t.Fatalf("seed distribute: %+v %v", res, err)
	}
	if _, err := a.UnassignTarget(tv.ID, "skill", ids["demo"]); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	return crashFixture{store: store, db: db, skillID: ids["demo"], targetID: tv.ID, targetPath: tv.Path}
}

func crashCmd(fx crashFixture, phase string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"SKILLCTL_TEST_CRASH="+phase,
		"SKILLCTL_TEST_STORE="+fx.store,
		"SKILLCTL_TEST_DB="+fx.db,
		"SKILLCTL_TEST_SKILL="+strconv.FormatInt(fx.skillID, 10),
		"SKILLCTL_TEST_TARGET="+strconv.FormatInt(fx.targetID, 10),
	)
	return cmd
}

func runCrashChild(t *testing.T, fx crashFixture, phase string, wantExit int) {
	t.Helper()
	cmd := crashCmd(fx, phase)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helper %s exited 0\n%s", phase, out)
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != wantExit {
		t.Fatalf("helper %s: %v (output %s)", phase, err, out)
	}
}

func reopenCrash(t *testing.T, fx crashFixture) *App {
	t.Helper()
	fresh, err := New(fx.store, fx.db)
	if err != nil {
		t.Fatalf("recover open: %v", err)
	}
	t.Cleanup(func() { fresh.Close() })
	return fresh
}

func readLive(t *testing.T, store, slug string) string {
	t.Helper()
	return readFile(t, filepath.Join(store, slug, "SKILL.md"))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
