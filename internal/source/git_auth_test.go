package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeRecorderGit writes a fake git executable that appends every
// invocation's argv to logPath and answers just enough of the protocol for
// observation wiring tests (ls-remote, clone, fetch, rev-parse, ls-tree,
// cat-file); every other command fails. Global options such as `-c key=value`
// and `-C dir` are skipped before the subcommand is dispatched. When
// RECORDER_CAT_MISSING is set, cat-file --batch reports every requested
// object as missing instead of serving content.
func writeRecorderGit(t *testing.T, logPath string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	content := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> \"" + logPath + "\"\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  case \"$1\" in\n" +
		"    -c) shift 2 ;;\n" +
		"    -C) shift 2 ;;\n" +
		"    -*) shift ;;\n" +
		"    *) break ;;\n" +
		"  esac\n" +
		"done\n" +
		"cmd=\"$1\"\n" +
		"shift\n" +
		"case \"$cmd\" in\n" +
		"  ls-remote) printf '0000000000000000000000000000000000000000\\tHEAD\\n' ;;\n" +
		"  clone) for last in \"$@\"; do :; done; mkdir -p \"$last\"; : > \"$last/HEAD\" ;;\n" +
		"  fetch) exit 0 ;;\n" +
		"  rev-parse) printf '0000000000000000000000000000000000000000\\n' ;;\n" +
		"  ls-tree) printf '100644 blob 0000000000000000000000000000000000000000\\tSKILL.md\\0' ;;\n" +
		"  cat-file)\n" +
		"    if [ \"$1\" = \"--batch\" ]; then\n" +
		"      while IFS= read -r oid; do\n" +
		"        if [ -n \"$RECORDER_CAT_MISSING\" ]; then\n" +
		"          printf '%s missing\\n' \"$oid\"\n" +
		"        else\n" +
		"          printf '%s blob 3\\nabc\\n' \"$oid\"\n" +
		"        fi\n" +
		"      done\n" +
		"    else\n" +
		"      printf 'tree\\n'\n" +
		"    fi ;;\n" +
		"  *) exit 1 ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

// envWithoutTokens returns the current environment with GH_TOKEN and
// GITHUB_TOKEN removed, so helper-behavior tests control the token source.
func envWithoutTokens() []string {
	var out []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GH_TOKEN=") || strings.HasPrefix(kv, "GITHUB_TOKEN=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGitAuthInjectsHelperForGitHubHTTPS(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "git.log")
	t.Setenv("SKILLCTL_GIT", writeRecorderGit(t, logPath))
	t.Setenv("GH_TOKEN", "ghp_top_secret_value")
	workDir := t.TempDir()
	loc := Locator{Kind: KindGit, Location: "https://github.com/owner/repo.git"}
	if _, err := runGitAuth(context.Background(), loc, "ls-remote", loc.Location); err != nil {
		t.Fatal(err)
	}
	log := readFile(t, logPath)
	if !strings.Contains(log, "credential.helper=!") {
		t.Fatalf("GitHub HTTPS ls-remote must carry the inline credential helper:\n%s", log)
	}
	if !strings.Contains(log, githubCredentialHelper) {
		t.Fatalf("the static helper command must be passed through git -c:\n%s", log)
	}
	if strings.Contains(log, "ghp_top_secret_value") {
		t.Fatalf("the token must never appear in git argv:\n%s", log)
	}
	if _, err := os.Stat(filepath.Join(workDir, "git-credential-helper.sh")); !os.IsNotExist(err) {
		t.Fatal("the credential helper must be inline, never a written file")
	}
}

func TestGitAuthSkipsHelperForInsecureOrNonGitHub(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "git.log")
	t.Setenv("SKILLCTL_GIT", writeRecorderGit(t, logPath))
	for _, loc := range []string{
		"http://github.com/owner/repo.git",
		"ssh://git@example.com/o/r.git",
		"git@example.com:o/r.git",
		"https://gitlab.com/o/r.git",
	} {
		if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		l := Locator{Kind: KindGit, Location: loc}
		if _, err := runGitAuth(context.Background(), l, "ls-remote", loc); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(readFile(t, logPath), "credential.helper=") {
			t.Fatalf("%s must not carry the GitHub helper:\n%s", loc, readFile(t, logPath))
		}
	}
}

// TestGitObserveGitHubHTTPSInjectsHelperWithoutSecrets drives the whole
// observation wiring for a GitHub HTTPS remote and proves every git
// invocation — including the cat-file --batch lazy blob fetch — carries
// the inline credential helper while the token never appears in any argv
// and no helper file is ever written.
func TestGitObserveGitHubHTTPSInjectsHelperWithoutSecrets(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "git.log")
	t.Setenv("SKILLCTL_GIT", writeRecorderGit(t, logPath))
	t.Setenv("GH_TOKEN", "ghp_top_secret_value")
	workDir := t.TempDir()
	loc, err := Normalize(KindGit, "github:owner/repo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	obs, err := (Git{}).Observe(context.Background(), loc, workDir)
	if err != nil {
		t.Fatal(err)
	}
	log := readFile(t, logPath)
	for _, want := range []string{"ls-remote", "clone", "ls-tree", "cat-file --batch", "credential.helper=!"} {
		if !strings.Contains(log, want) {
			t.Fatalf("observation log missing %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "ghp_top_secret_value") {
		t.Fatalf("the token must never appear in any git argv:\n%s", log)
	}
	if _, err := os.Stat(filepath.Join(workDir, "git-credential-helper.sh")); !os.IsNotExist(err) {
		t.Fatal("the credential helper must be inline, never a written file")
	}
	if len(obs.Entries) != 0 || len(obs.Issues) != 1 {
		t.Fatalf("the fake blob must surface as one invalid-SKILL.md issue: %+v", obs)
	}
}

// TestFetchTreeBlobsUsesHelperOnlyForGitHubHTTPS is the focused regression
// for the lazy blob fetch: cat-file --batch receives the same ambient
// credential helper as every other network invocation, and only for GitHub
// HTTPS remotes, with the token never in argv.
func TestFetchTreeBlobsUsesHelperOnlyForGitHubHTTPS(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "git.log")
	t.Setenv("SKILLCTL_GIT", writeRecorderGit(t, logPath))
	t.Setenv("GH_TOKEN", "ghp_top_secret_value")
	workDir := t.TempDir()
	rows := []treeLine{
		{mode: "100644", typ: "blob", oid: "0000000000000000000000000000000000000001", path: "SKILL.md"},
		{mode: "040000", typ: "tree", oid: "0000000000000000000000000000000000000002", path: "refs"},
	}

	blobs, err := fetchTreeBlobs(context.Background(), workDir, rows,
		Locator{Kind: KindGit, Location: "https://github.com/owner/repo.git"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(blobs["0000000000000000000000000000000000000001"]); got != "abc" {
		t.Fatalf("blob content: got %q", got)
	}
	if len(blobs) != 1 {
		t.Fatalf("tree rows must not be requested as blobs: %v", blobs)
	}
	log := readFile(t, logPath)
	if !strings.Contains(log, "cat-file --batch") || !strings.Contains(log, "credential.helper=!") {
		t.Fatalf("GitHub HTTPS batch fetch must carry the inline helper:\n%s", log)
	}
	if strings.Contains(log, "ghp_top_secret_value") {
		t.Fatalf("the token must never appear in git argv:\n%s", log)
	}

	// a non-GitHub remote keeps plain git behavior: no helper at all.
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if _, err := fetchTreeBlobs(context.Background(), workDir, rows,
		Locator{Kind: KindGit, Location: "file:///tmp/remote.git"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, logPath), "credential.helper=") {
		t.Fatalf("non-GitHub batch fetch must not carry the helper:\n%s", readFile(t, logPath))
	}
}

// TestFetchTreeBlobsMissingErrorHidesSecrets proves a failed lazy fetch is
// reported without ever echoing the ambient token.
func TestFetchTreeBlobsMissingErrorHidesSecrets(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "git.log")
	t.Setenv("SKILLCTL_GIT", writeRecorderGit(t, logPath))
	t.Setenv("GH_TOKEN", "ghp_top_secret_value")
	t.Setenv("RECORDER_CAT_MISSING", "1")
	rows := []treeLine{{mode: "100644", typ: "blob", oid: "0000000000000000000000000000000000000001", path: "SKILL.md"}}
	_, err := fetchTreeBlobs(context.Background(), t.TempDir(), rows,
		Locator{Kind: KindGit, Location: "https://github.com/owner/repo.git"})
	if err == nil {
		t.Fatal("missing object: want error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error must name the failure: %v", err)
	}
	if strings.Contains(err.Error(), "ghp_top_secret_value") {
		t.Fatalf("the token must never appear in errors: %v", err)
	}
}

// runCredentialHelper executes the inline credential helper command the
// same way git does: git appends the action to the command text and runs
// `sh -c '<command> <action>' '<command> <action>'`, so the trailing `f`
// invocation receives the action as $1.
func runCredentialHelper(t *testing.T, env []string, args ...string) string {
	t.Helper()
	cmd := strings.TrimPrefix(githubCredentialHelper, "!")
	text := strings.TrimSpace(cmd + " " + strings.Join(args, " "))
	c := exec.Command("sh", "-c", text, text)
	c.Env = env
	out, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestCredentialHelperReadsEnvToken(t *testing.T) {
	out := runCredentialHelper(t, append(envWithoutTokens(), "GH_TOKEN=ghp_env_secret_value"), "get")
	if !strings.Contains(out, "username=x-access-token") || !strings.Contains(out, "password=ghp_env_secret_value") {
		t.Fatalf("env token not supplied by the helper: %q", out)
	}
}

func TestCredentialHelperUsesGhSession(t *testing.T) {
	ghDir := t.TempDir()
	ghLog := filepath.Join(ghDir, "gh.log")
	gh := filepath.Join(ghDir, "gh")
	content := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + ghLog + "\"\nprintf 'gh_session_secret_value\\n'\n"
	if err := os.WriteFile(gh, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	out := runCredentialHelper(t, append(envWithoutTokens(), "PATH="+ghDir+string(os.PathListSeparator)+os.Getenv("PATH")), "get")
	if !strings.Contains(out, "password=gh_session_secret_value") {
		t.Fatalf("gh session token not supplied by the helper: %q", out)
	}
	if log := readFile(t, ghLog); !strings.Contains(log, "auth token") {
		t.Fatalf("gh must be invoked with `auth token`: %q", log)
	}
}

func TestCredentialHelperEmptyWithoutAuth(t *testing.T) {
	out := runCredentialHelper(t, append(envWithoutTokens(), "PATH="+t.TempDir()), "get")
	if strings.TrimSpace(out) != "" {
		t.Fatalf("without any token source the helper must print nothing: %q", out)
	}
}

// TestCredentialHelperIgnoresStoreAndErase proves the helper only answers
// `get`: store/erase requests (which git sends after successful use) print
// nothing, so the ambient token is never persisted by git.
func TestCredentialHelperIgnoresStoreAndErase(t *testing.T) {
	for _, action := range []string{"store", "erase"} {
		out := runCredentialHelper(t, append(envWithoutTokens(), "GH_TOKEN=ghp_env_secret_value"), action)
		if strings.TrimSpace(out) != "" {
			t.Fatalf("%s must print nothing: %q", action, out)
		}
	}
}
