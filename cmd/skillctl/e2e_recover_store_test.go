package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"skillctl/internal/app"
)

func snapshotStore(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = "symlink:" + target
			return nil
		}
		if d.IsDir() {
			out[filepath.ToSlash(rel)] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertStoreUnchanged(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("store tree size changed: before %d after %d", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("store path %s rewritten: %q -> %q", p, h, after[p])
		}
	}
}

func decodeItems(t *testing.T, stdout string) (int, []map[string]any) {
	t.Helper()
	var env struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	return env.Total, env.Items
}

// TestCLIRecoverStoreEndToEnd proves state.db loss recovery through a
// real skillctl process: valid top-level Store directories become unbound
// Skills, relationships are gone, leftover internal trees stay, and no
// Store content is rewritten.
func TestCLIRecoverStoreEndToEnd(t *testing.T) {
	home := t.TempDir()
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
		t.Fatalf("import: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "group", "create", "crew"); code != 0 {
		t.Fatalf("group: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "group", "add-skill", "crew", "alpha"); code != 0 {
		t.Fatalf("group add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "add", "--path", "~/skills", "--name", "dist"); code != 0 {
		t.Fatalf("target add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "dist", "--skill", "alpha"); code != 0 {
		t.Fatalf("assign: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "distribute", "dist"); code != 0 {
		t.Fatalf("distribute: exit %d\n%s", code, out)
	}
	link := filepath.Join(home, "skills", "alpha")
	raw, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotStore(t, store)

	if err := os.Remove(filepath.Join(home, ".skillctl", "state.db")); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, home, "status")
	if code != 0 || !strings.Contains(out, "Status: state_missing") {
		t.Fatalf("status after loss: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "init")
	if code != 1 {
		t.Fatalf("plain init on state_missing: exit %d\n%s", code, out)
	}

	stdout, stderr, code := runOut(t, home, "init", "--recover-store", "--json")
	if code != 0 {
		t.Fatalf("recover-store: exit %d\n%s%s", code, stdout, stderr)
	}
	var result app.StoreRecoveryResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("recover json: %v\n%s", err, stdout)
	}
	if len(result.Recovered) != 2 {
		t.Fatalf("recovered: %+v", result)
	}
	if len(result.PreservedInternal) == 0 {
		t.Fatal("expected leftover Baseline/previous trees to be preserved")
	}
	assertStoreUnchanged(t, before, snapshotStore(t, store))

	stdout, stderr, code = runOut(t, home, "skill", "list", "--json")
	if code != 0 {
		t.Fatalf("skill list: exit %d\n%s%s", code, stdout, stderr)
	}
	total, items := decodeItems(t, stdout)
	if total != 2 {
		t.Fatalf("skills: %s", stdout)
	}
	for _, it := range items {
		if it["sync_status"] != "unbound" || it["binding"] != nil {
			t.Fatalf("skill must be unbound: %v", it)
		}
		if it["has_previous_snapshot"] == true {
			t.Fatalf("must not invent snapshots: %v", it)
		}
	}

	for _, noun := range []string{"source", "group", "target"} {
		stdout, stderr, code = runOut(t, home, noun, "list", "--json")
		if code != 0 {
			t.Fatalf("%s list: exit %d\n%s%s", noun, code, stdout, stderr)
		}
		n, _ := decodeItems(t, stdout)
		if n != 0 {
			t.Fatalf("%s list must be empty after recover: %s", noun, stdout)
		}
	}

	if got, err := os.Readlink(link); err != nil || got != raw {
		t.Fatalf("existing Target link changed: %q %v (was %q)", got, err, raw)
	}

	if out, code := run(t, home, "target", "add", "--path", "~/skills", "--name", "again"); code != 0 {
		t.Fatalf("re-add target: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "target", "assign", "again", "--skill", "alpha"); code != 0 {
		t.Fatalf("assign after recover: exit %d\n%s", code, out)
	}
	stdout, stderr, code = runOut(t, home, "target", "status", "again", "--refresh", "--json")
	if code != 0 {
		t.Fatalf("status after re-add: exit %d\n%s%s", code, stdout, stderr)
	}
	var st e2eDistributionStatus
	if err := json.Unmarshal([]byte(stdout), &st); err != nil {
		t.Fatalf("status json: %v\n%s", err, stdout)
	}
	if len(st.Items) != 1 || st.Items[0].Managed || st.Items[0].Observed != "conflict" {
		t.Fatalf("leftover link must stay unmanaged: %+v", st)
	}
	if got, err := os.Readlink(link); err != nil || got != raw {
		t.Fatalf("inspect must not rewrite the leftover link: %q %v", got, err)
	}
}

func TestUIRecoverStoreEndToEnd(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}
	root := t.TempDir()
	writeE2ESkill(t, root, "alpha")
	if out, code := run(t, home, "source", "add", root, "--name", "ui-local"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run(t, home, "skill", "import", "--source", "ui-local", "--all"); code != 0 {
		t.Fatalf("import: exit %d\n%s", code, out)
	}
	before := snapshotStore(t, store)
	if err := os.Remove(filepath.Join(home, ".skillctl", "state.db")); err != nil {
		t.Fatal(err)
	}

	up := startUI(t, home)
	client := &http.Client{Timeout: 5 * time.Second}
	resp := get(t, client, up.url+"/api/v1/status")
	var st struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if st.State != "state_missing" {
		t.Fatalf("ui status: %+v", st)
	}

	resp = post(t, client, up.url+"/api/v1/setup", `{}`)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("setup without recover: %d %s", resp.StatusCode, b)
	}

	resp = post(t, client, up.url+"/api/v1/setup", `{"recover_store": true}`)
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recover setup: %d %s", resp.StatusCode, b)
	}
	var got app.StoreRecoveryResult
	if err := json.Unmarshal(b, &got); err != nil || len(got.Recovered) != 1 || got.Recovered[0].Slug != "alpha" {
		t.Fatalf("recover body: %s", b)
	}
	assertStoreUnchanged(t, before, snapshotStore(t, store))

	resp = get(t, client, up.url+"/api/v1/skills")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("skills: %d %s", resp.StatusCode, b)
	}
	total, items := decodeItems(t, string(b))
	if total != 1 || items[0]["sync_status"] != "unbound" {
		t.Fatalf("skills after recover: %s", b)
	}
}
