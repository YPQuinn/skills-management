package source

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// gitCommandTimeout bounds every git invocation so a stalled network never
// blocks a check indefinitely.
const gitCommandTimeout = 2 * time.Minute

var fullSHARe = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// Git observes Git Sources through ambient credentials only: it invokes the
// user's git with its normal environment (SSH agent, credential helpers, gh
// configured helpers) and never stores or passes tokens itself.
type Git struct{}

// Observe resolves the Source ref to a commit, ensures that commit is in
// the per-location cache, and discovers Skills with complete-tree digests.
func (Git) Observe(ctx context.Context, loc Locator, workDir string) (Observation, error) {
	return gitObserve(ctx, loc, workDir, discoverFull)
}

// ObserveListing is the Git registration scan: SKILL.md frontmatter only,
// no complete-tree blob download. Import and check use Observe.
func (Git) ObserveListing(ctx context.Context, loc Locator, workDir string) (Observation, error) {
	return gitObserve(ctx, loc, workDir, discoverListing)
}

func gitObserve(ctx context.Context, loc Locator, workDir string, mode discoverMode) (Observation, error) {
	if loc.Kind != KindGit {
		return Observation{}, fmt.Errorf("Git observer requires a Git locator")
	}
	start := time.Now()
	commit, err := ResolveCommit(ctx, loc)
	if err != nil {
		return Observation{}, err
	}
	tracef("resolve_commit %s %s", shortSHA(commit), time.Since(start).Round(time.Millisecond))
	cache := gitCacheDir(workDir, loc.Location)
	ensured := time.Now()
	if err := ensureCommit(ctx, loc, cache, commit); err != nil {
		return Observation{}, err
	}
	tracef("ensure_commit %s", time.Since(ensured).Round(time.Millisecond))
	scanned := time.Now()
	obs, err := discoverCommit(ctx, cache, commit, loc, mode)
	if err != nil {
		return Observation{}, err
	}
	tracef("discover mode=%s entries=%d issues=%d %s", mode, len(obs.Entries), len(obs.Issues), time.Since(scanned).Round(time.Millisecond))
	obs.Commit = commit
	return obs, nil
}

// ResolveCommit resolves the Source ref to a full commit SHA: the remote
// default branch when no ref is given, the named branch or tag otherwise. A
// full commit SHA is pinned and returned unchanged (reachability is verified
// when the cache is filled).
func ResolveCommit(ctx context.Context, loc Locator) (string, error) {
	if isFullSHA(loc.Ref) {
		return strings.ToLower(loc.Ref), nil
	}
	args := []string{"ls-remote", loc.Location}
	if loc.Ref == "" {
		args = append(args, "HEAD")
	} else {
		args = append(args,
			"refs/heads/"+loc.Ref,
			"refs/tags/"+loc.Ref,
			"refs/tags/"+loc.Ref+"^{}",
			loc.Ref,
		)
	}
	out, err := runGitAuth(ctx, loc, args...)
	if err != nil {
		return "", fmt.Errorf("checking %s: %v", loc.Location, err)
	}
	// A peeled annotated tag (line ending in ^{}) names the commit; prefer
	// it over the tag object SHA.
	var first, peeled string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if first == "" {
			first = fields[0]
		}
		if strings.HasSuffix(fields[len(fields)-1], "^{}") {
			peeled = fields[0]
		}
	}
	if peeled != "" {
		return peeled, nil
	}
	if first != "" {
		return first, nil
	}
	if loc.Ref == "" {
		return "", fmt.Errorf("repository %s has no default branch", loc.Location)
	}
	return "", fmt.Errorf("ref %q not found in %s", loc.Ref, loc.Location)
}

func isFullSHA(ref string) bool {
	return fullSHARe.MatchString(ref)
}

// gitBin returns the git executable; SKILLCTL_GIT overrides it for tests and
// fake-executable fixtures.
func gitBin() string {
	if bin := os.Getenv("SKILLCTL_GIT"); bin != "" {
		return bin
	}
	return "git"
}

// runGit runs one git command with ambient credentials and never prompts for
// terminal input, so authentication failures surface immediately instead of
// hanging.
func runGit(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitBin(), args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	start := time.Now()
	err := cmd.Run()
	traceGit(args, time.Since(start), err)
	if err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out.String(), nil
}
