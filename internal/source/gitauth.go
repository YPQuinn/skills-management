package source

import (
	"context"
	"strings"
)

// githubHTTPS reports whether loc is a secure GitHub HTTPS remote, the only
// remotes that use the ambient GitHub credential helper. Plain HTTP must
// never receive a token; every other remote keeps plain git behavior.
func githubHTTPS(loc Locator) bool {
	return strings.HasPrefix(loc.Location, "https://github.com/")
}

// githubCredentialHelper is a static inline credential helper shell command
// injected through `git -c credential.helper=!...` for GitHub HTTPS remotes.
// It contains no secrets and reads them only from the environment at
// runtime, so no helper file is ever written (no creation races, symlink
// execution, or paths-with-spaces bugs) and token values never enter
// locator fields, command arguments, logs, errors, or cache keys. GH_TOKEN
// and GITHUB_TOKEN take precedence; otherwise an authenticated `gh auth
// token` session is consulted. When no token is available the helper prints
// nothing and git proceeds with the user's normal credential helpers or
// public access. Git appends the action to the command text and runs it as
// `sh -c '<command> <action>' '<command> <action>'`, so the constant must
// end with the bare `f` invocation that receives the action as $1.
// Anything the helper prints on stdout goes to git's credential plumbing,
// never to our logs or output.
const githubCredentialHelper = `!f() { if [ "$1" = "get" ]; then token=""; if [ -n "$GH_TOKEN" ]; then token="$GH_TOKEN"; elif [ -n "$GITHUB_TOKEN" ]; then token="$GITHUB_TOKEN"; elif command -v gh >/dev/null 2>&1; then token=$(gh auth token 2>/dev/null) || token=""; fi; if [ -n "$token" ]; then printf 'username=x-access-token\npassword=%s\n' "$token"; fi; fi; }; f`

// gitAuthArgs returns the global git arguments that inject the ambient
// GitHub credential helper for GitHub HTTPS remotes, and nil for every
// other remote so plain git credential behavior is untouched.
func gitAuthArgs(loc Locator) []string {
	if !githubHTTPS(loc) {
		return nil
	}
	return []string{"-c", "credential.helper=" + githubCredentialHelper}
}

// runGitAuth runs one git command that may need network credentials,
// injecting the environment-only GitHub credential helper for GitHub HTTPS
// remotes through `git -c` for that invocation only. The helper is never
// written into the cache or persisted configuration, so token values never
// enter locator fields, command arguments, logs, errors, or cache keys.
func runGitAuth(ctx context.Context, loc Locator, args ...string) (string, error) {
	return runGit(ctx, append(gitAuthArgs(loc), args...)...)
}
