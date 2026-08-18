package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"skillctl/internal/lock"
)

func e2eSourceID(t *testing.T, home string) int64 {
	t.Helper()
	stdout, stderr, code := runOut(t, home, "source", "list", "--json")
	if code != 0 {
		t.Fatalf("source list: exit %d\n%s%s", code, stdout, stderr)
	}
	var env struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || len(env.Items) != 1 {
		t.Fatalf("source list: %v %s", err, stdout)
	}
	return env.Items[0].ID
}

// Crash-window process tests live in internal/app/crash_process_test.go:
// a test-only helper child terminates at commit/link hooks. Planting
// journal or filesystem state here is not the process-level proof.

func TestProcessStoreLockRefusesSecondWriterThenSucceeds(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "alpha")
	if out, code := run(t, home, "source", "add", root, "--name", "lock-src"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	lockPath := filepath.Join(home, ".skillctl", "locks", "store.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	out, code := run(t, home, "skill", "import", "--source", "lock-src", "--all")
	if code != 1 || !strings.Contains(out, "another skillctl process") {
		t.Fatalf("import under store lock: exit %d\n%s", code, out)
	}
	if err := held.Unlock(); err != nil {
		t.Fatal(err)
	}
	if out, code := run(t, home, "skill", "import", "--source", "lock-src", "--all"); code != 0 {
		t.Fatalf("import after unlock: exit %d\n%s", code, out)
	}
}

func TestProcessTargetLockRefusesThenSucceeds(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "demo")
	if out, code := run(t, home, "source", "add", root, "--name", "lock-src"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "lock-src", "--all"); code != 0 {
		t.Fatalf("import: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", "~/skills", "--name", "dist"); code != 0 {
		t.Fatalf("target add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "dist", "--skill", "demo"); code != 0 {
		t.Fatalf("assign: exit %d\n%s", code, out)
	}
	stdout, stderr, code := runOut(t, home, "target", "show", "dist", "--json")
	if code != 0 {
		t.Fatalf("target show: exit %d\n%s%s", code, stdout, stderr)
	}
	var tv struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &tv); err != nil || tv.ID == 0 {
		t.Fatalf("target json: %v %s", err, stdout)
	}
	lockPath := filepath.Join(home, ".skillctl", "locks", "targets", fmt.Sprintf("%d.lock", tv.ID))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	out, code := run(t, home, "target", "distribute", "dist")
	if code != 1 || !strings.Contains(out, "another skillctl process") {
		t.Fatalf("distribute under target lock: exit %d\n%s", code, out)
	}
	if err := held.Unlock(); err != nil {
		t.Fatal(err)
	}
	if out, code := run(t, home, "target", "distribute", "dist"); code != 0 {
		t.Fatalf("distribute after unlock: exit %d\n%s", code, out)
	}
}

func TestProcessConcurrentDistinctTargets(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "demo")
	if out, code := run(t, home, "source", "add", root, "--name", "lock-src"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "lock-src", "--all"); code != 0 {
		t.Fatalf("import: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", filepath.Join(home, "t1"), "--name", "one"); code != 0 {
		t.Fatalf("target one: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", filepath.Join(home, "t2"), "--name", "two"); code != 0 {
		t.Fatalf("target two: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "one", "--skill", "demo"); code != 0 {
		t.Fatalf("assign one: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "two", "--skill", "demo"); code != 0 {
		t.Fatalf("assign two: exit %d\n%s", code, out)
	}

	var wg sync.WaitGroup
	codes := make([]int, 2)
	outs := make([]string, 2)
	runOne := func(i int, name string) {
		defer wg.Done()
		outs[i], codes[i] = run(t, home, "target", "distribute", name)
	}
	wg.Add(2)
	go runOne(0, "one")
	go runOne(1, "two")
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent distributes deadlocked")
	}
	succeeded := 0
	for i, name := range []string{"one", "two"} {
		switch codes[i] {
		case 0:
			succeeded++
			if _, err := os.Lstat(filepath.Join(home, map[string]string{"one": "t1", "two": "t2"}[name], "demo")); err != nil {
				t.Fatalf("successful distribute %s left no link: %v", name, err)
			}
		case 1:
			if !strings.Contains(outs[i], "another skillctl process") {
				t.Fatalf("distribute %s: exit %d\n%s", name, codes[i], outs[i])
			}
			if _, err := os.Lstat(filepath.Join(home, map[string]string{"one": "t1", "two": "t2"}[name], "demo")); err == nil {
				t.Fatalf("locked distribute %s must not leave a partial link", name)
			}
		default:
			t.Fatalf("distribute %s: exit %d\n%s", name, codes[i], outs[i])
		}
	}
	if succeeded == 0 {
		t.Fatalf("at least one distinct-Target CLI distribute must complete: %q %q", outs[0], outs[1])
	}
}

func TestProcessCLIandRESTSerializeOnStoreLock(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "alpha")
	if out, code := run(t, home, "source", "add", root, "--name", "lock-src"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	srcID := e2eSourceID(t, home)
	up := startUI(t, home)
	client := &http.Client{Timeout: 5 * time.Second}

	lockPath := filepath.Join(home, ".skillctl", "locks", "store.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"source_id": %d, "all": true}`, srcID)
	resp := post(t, client, up.url+"/api/v1/skills/import", body)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("REST import under lock: %d %s", resp.StatusCode, b)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &env); err != nil || env.Error.Code != "locked" {
		t.Fatalf("REST locked envelope: %s", b)
	}
	out, code := run(t, home, "skill", "import", "--source", "lock-src", "--all")
	if code != 1 || !strings.Contains(out, "another skillctl process") {
		t.Fatalf("CLI import under lock: exit %d\n%s", code, out)
	}
	if err := held.Unlock(); err != nil {
		t.Fatal(err)
	}
	if out, code := run(t, home, "skill", "import", "--source", "lock-src", "--all"); code != 0 {
		t.Fatalf("CLI import after unlock: exit %d\n%s", code, out)
	}
}

func TestProcessRESTConcurrentDistinctTargets(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "demo")
	if out, code := run(t, home, "source", "add", root, "--name", "lock-src"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "lock-src", "--all"); code != 0 {
		t.Fatalf("import: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", filepath.Join(home, "t1"), "--name", "one"); code != 0 {
		t.Fatalf("target one: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", filepath.Join(home, "t2"), "--name", "two"); code != 0 {
		t.Fatalf("target two: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "one", "--skill", "demo"); code != 0 {
		t.Fatalf("assign one: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "two", "--skill", "demo"); code != 0 {
		t.Fatalf("assign two: exit %d\n%s", code, out)
	}
	stdout, stderr, code := runOut(t, home, "target", "list", "--json")
	if code != 0 {
		t.Fatalf("target list: exit %d\n%s%s", code, stdout, stderr)
	}
	var list struct {
		Items []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(stdout), &list); err != nil || len(list.Items) != 2 {
		t.Fatalf("targets: %v %s", err, stdout)
	}

	up := startUI(t, home)
	client := &http.Client{Timeout: 15 * time.Second}
	var wg sync.WaitGroup
	status := make([]int, 2)
	bodies := make([]string, 2)
	runOne := func(i int, id int64) {
		defer wg.Done()
		resp := post(t, client, fmt.Sprintf("%s/api/v1/targets/%d/distribute", up.url, id), `{"dry_run": false}`)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		status[i] = resp.StatusCode
		bodies[i] = string(b)
	}
	wg.Add(2)
	go runOne(0, list.Items[0].ID)
	go runOne(1, list.Items[1].ID)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("REST concurrent distributes deadlocked")
	}
	if status[0] != http.StatusOK || status[1] != http.StatusOK {
		t.Fatalf("REST concurrent distributes: %d %s\n%d %s", status[0], bodies[0], status[1], bodies[1])
	}
	for _, name := range []string{"t1", "t2"} {
		if _, err := os.Lstat(filepath.Join(home, name, "demo")); err != nil {
			t.Fatalf("link missing at %s: %v", name, err)
		}
	}
}

func TestProcessCLIandRESTSimultaneousMutation(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "demo")
	writeE2ESkill(t, root, "other")
	if out, code := run(t, home, "source", "add", root, "--name", "lock-src"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "lock-src", "--all"); code != 0 {
		t.Fatalf("import: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", filepath.Join(home, "t1"), "--name", "one"); code != 0 {
		t.Fatalf("target one: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", filepath.Join(home, "t2"), "--name", "two"); code != 0 {
		t.Fatalf("target two: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "one", "--skill", "demo"); code != 0 {
		t.Fatalf("assign one: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "two", "--skill", "other"); code != 0 {
		t.Fatalf("assign two: exit %d\n%s", code, out)
	}
	stdout, stderr, code := runOut(t, home, "target", "show", "one", "--json")
	if code != 0 {
		t.Fatalf("target show: exit %d\n%s%s", code, stdout, stderr)
	}
	var one struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &one); err != nil || one.ID == 0 {
		t.Fatalf("target json: %v %s", err, stdout)
	}

	up := startUI(t, home)
	client := &http.Client{Timeout: 15 * time.Second}
	var wg sync.WaitGroup
	var restStatus int
	var restBody, cliOut string
	var cliCode int
	wg.Add(2)
	go func() {
		defer wg.Done()
		resp := post(t, client, fmt.Sprintf("%s/api/v1/targets/%d/distribute", up.url, one.ID), `{"dry_run": false}`)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		restStatus = resp.StatusCode
		restBody = string(b)
	}()
	go func() {
		defer wg.Done()
		cliOut, cliCode = run(t, home, "target", "distribute", "two")
	}()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("CLI+REST simultaneous mutation deadlocked")
	}
	restOK := restStatus == http.StatusOK
	restLocked := restStatus == http.StatusConflict && strings.Contains(restBody, "locked")
	cliOK := cliCode == 0
	cliLocked := cliCode == 1 && strings.Contains(cliOut, "another skillctl process")
	if !restOK && !restLocked {
		t.Fatalf("REST distribute: %d %s", restStatus, restBody)
	}
	if !cliOK && !cliLocked {
		t.Fatalf("CLI distribute: exit %d\n%s", cliCode, cliOut)
	}
	if !restOK && !cliOK {
		t.Fatal("simultaneous CLI+REST must complete at least one mutation")
	}
	if restOK {
		if _, err := os.Lstat(filepath.Join(home, "t1", "demo")); err != nil {
			t.Fatalf("REST success left no link: %v", err)
		}
	} else if _, err := os.Lstat(filepath.Join(home, "t1", "demo")); err == nil {
		t.Fatal("locked REST must not leave a partial link")
	}
	if cliOK {
		if _, err := os.Lstat(filepath.Join(home, "t2", "other")); err != nil {
			t.Fatalf("CLI success left no link: %v", err)
		}
	} else if _, err := os.Lstat(filepath.Join(home, "t2", "other")); err == nil {
		t.Fatal("locked CLI must not leave a partial link")
	}
}

func TestProcessSameTargetDoesNotInterleave(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "demo")
	if out, code := run(t, home, "source", "add", root, "--name", "lock-src"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "lock-src", "--all"); code != 0 {
		t.Fatalf("import: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", filepath.Join(home, "t1"), "--name", "one"); code != 0 {
		t.Fatalf("target: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "one", "--skill", "demo"); code != 0 {
		t.Fatalf("assign: exit %d\n%s", code, out)
	}
	stdout, stderr, code := runOut(t, home, "target", "show", "one", "--json")
	if code != 0 {
		t.Fatalf("target show: exit %d\n%s%s", code, stdout, stderr)
	}
	var tv struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &tv); err != nil || tv.ID == 0 {
		t.Fatalf("target json: %v %s", err, stdout)
	}

	up := startUI(t, home)
	client := &http.Client{Timeout: 15 * time.Second}
	var wg sync.WaitGroup
	status := make([]int, 2)
	bodies := make([]string, 2)
	runOne := func(i int) {
		defer wg.Done()
		resp := post(t, client, fmt.Sprintf("%s/api/v1/targets/%d/distribute", up.url, tv.ID), `{"dry_run": false}`)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		status[i] = resp.StatusCode
		bodies[i] = string(b)
	}
	wg.Add(2)
	go runOne(0)
	go runOne(1)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("same-Target distributes deadlocked")
	}
	ok, locked := 0, 0
	for i := range status {
		switch {
		case status[i] == http.StatusOK:
			ok++
		case status[i] == http.StatusConflict && strings.Contains(bodies[i], "locked"):
			locked++
		default:
			t.Fatalf("same-Target distribute %d: %d %s", i, status[i], bodies[i])
		}
	}
	if ok != 1 || locked != 1 {
		t.Fatalf("same Target must serialize to one success and one lock: %d %s\n%d %s", status[0], bodies[0], status[1], bodies[1])
	}
	info, err := os.Lstat(filepath.Join(home, "t1", "demo"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("same-Target success must leave exactly one symlink: %v", err)
	}
}
