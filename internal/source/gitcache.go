package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"skillctl/internal/lock"
)

// ensureCommit makes commit reachable in the per-location bare cache.
// If the cache already has that commit, it does not talk to the remote.
// Otherwise it clones a single branch (no tags) or fetches only that
// commit. It never fetches every branch and tag, and it does not use a
// blobless partial clone: lazy promisor fetches open one HTTPS connection
// per missing blob and fail under LibreSSL against GitHub.
func ensureCommit(ctx context.Context, loc Locator, cache, commit string) error {
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return err
	}
	held, err := lock.TryExclusive(cache + ".lock")
	if err != nil {
		return err
	}
	defer held.Unlock()

	if cacheExists(cache) {
		if commitPresent(ctx, cache, commit) {
			tracef("cache hit %s", commit[:min(12, len(commit))])
			return nil
		}
		if err := fetchCommit(ctx, loc, cache, commit); err != nil {
			return fmt.Errorf("refreshing %s: %v", loc.Location, err)
		}
		return nil
	}
	if err := cloneBare(ctx, loc, cache); err != nil {
		return err
	}
	if commitPresent(ctx, cache, commit) {
		return nil
	}
	if err := fetchCommit(ctx, loc, cache, commit); err != nil {
		return fmt.Errorf("refreshing %s: %v", loc.Location, err)
	}
	return nil
}

func cacheExists(cache string) bool {
	_, err := os.Stat(filepath.Join(cache, "HEAD"))
	return err == nil
}

func commitPresent(ctx context.Context, cache, commit string) bool {
	_, err := runGit(ctx, "-C", cache, "rev-parse", "--verify", "--quiet", commit+"^{commit}")
	return err == nil
}

func cloneBare(ctx context.Context, loc Locator, cache string) error {
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return err
	}
	start := time.Now()
	args := []string{"clone", "--bare", "--single-branch", "--no-tags"}
	if ref := loc.Ref; ref != "" && !isFullSHA(ref) && !strings.Contains(ref, "/") {
		args = append(args, "--branch", ref)
	}
	args = append(args, loc.Location, cache)
	if _, err := runGitAuth(ctx, loc, args...); err != nil {
		os.RemoveAll(cache)
		return fmt.Errorf("cloning %s: %v", loc.Location, err)
	}
	tracef("clone %s", time.Since(start).Round(time.Millisecond))
	return nil
}

func fetchCommit(ctx context.Context, loc Locator, cache, commit string) error {
	start := time.Now()
	if _, err := runGitAuth(ctx, loc, "-C", cache, "fetch", "--no-tags", "origin", commit); err != nil {
		return err
	}
	if !commitPresent(ctx, cache, commit) {
		return fmt.Errorf("commit %s is not reachable in the repository", commit)
	}
	tracef("fetch %s %s", commit[:min(12, len(commit))], time.Since(start).Round(time.Millisecond))
	return nil
}

// gitCacheDir is the per-location cache directory: a content-addressed key
// derived from the normalized location, so one cache serves every check of
// that location and different locations never share a cache.
func gitCacheDir(workDir, location string) string {
	sum := sha256.Sum256([]byte(location))
	return filepath.Join(workDir, "git-cache", hex.EncodeToString(sum[:8]))
}
