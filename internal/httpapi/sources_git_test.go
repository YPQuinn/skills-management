package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// makeBareRemote builds a working Git repository with the given files and
// returns its bare clone path and working directory, skipping the test when
// git is unavailable.
func makeBareRemote(t *testing.T, files map[string]string) (bare, work string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	work = t.TempDir()
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run(work, "init", "-b", "main")
	run(work, "config", "user.email", "test@example.com")
	run(work, "config", "user.name", "Test")
	for path, content := range files {
		full := filepath.Join(work, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(work, "add", ".")
	run(work, "commit", "-m", "one")
	bare = filepath.Join(t.TempDir(), "remote.git")
	run(t.TempDir(), "clone", "--bare", work, bare)
	return bare, work
}

func TestSourcesRESTGitLifecycle(t *testing.T) {
	bare, work := makeBareRemote(t, map[string]string{
		"skills/alpha/SKILL.md": "---\nname: Alpha\ndescription: one\n---\n",
	})
	ts := readyServer(t, nil)

	// registration resolves the remote and records the commit
	body := fmt.Sprintf(`{"kind": "git", "location": %q, "name": "git-one"}`, "file://"+bare)
	resp := ts.do(t, "POST", "/api/v1/sources", body, "application/json", "", "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create git: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var created sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &created); err != nil {
		t.Fatal(err)
	}
	if created.Kind != "git" || len(created.LastCommit) != 40 || created.EntryCount != 1 || created.Inventory[0].Name != "Alpha" {
		t.Fatalf("created git: %+v", created)
	}

	// the detail resource presents the resolved commit
	resp = ts.do(t, "GET", fmt.Sprintf("/api/v1/sources/%d", created.ID), "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("show git: got %d", resp.StatusCode)
	}
	var shown sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.LastCommit != created.LastCommit {
		t.Fatalf("show commit: got %q, want %q", shown.LastCommit, created.LastCommit)
	}

	// an upstream advance moves the observed commit on check
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	writeAPISkill(t, work, "beta")
	run(work, "add", ".")
	run(work, "commit", "-m", "two")
	run(work, "push", bare, "main")

	resp = ts.do(t, "POST", fmt.Sprintf("/api/v1/sources/%d/check", created.ID), `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check git: got %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var checked sourceJSON
	if err := json.Unmarshal([]byte(readBody(t, resp)), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.LastCommit == created.LastCommit || checked.EntryCount != 2 {
		t.Fatalf("git check did not observe the update: %+v (was %+v)", checked, created)
	}
}
