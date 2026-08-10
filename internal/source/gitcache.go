package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"skillctl/internal/lock"
)

// ensureCache clones or refreshes the bare partial clone for one normalized
// location. The per-cache lock serializes checks across processes; a held
// lock fails immediately as ErrLocked.
func ensureCache(ctx context.Context, loc Locator, cache string) error {
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return err
	}
	held, err := lock.TryExclusive(cache + ".lock")
	if err != nil {
		return err
	}
	defer held.Unlock()

	if _, err := os.Stat(filepath.Join(cache, "HEAD")); err == nil {
		_, err := runGitAuth(ctx, loc, "-C", cache, "fetch", "--prune", "origin",
			"+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*")
		if err != nil {
			return fmt.Errorf("refreshing %s: %v", loc.Location, err)
		}
		return nil
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return err
	}
	if _, err := runGitAuth(ctx, loc, "clone", "--bare", "--filter=blob:none", loc.Location, cache); err != nil {
		os.RemoveAll(cache)
		return fmt.Errorf("cloning %s: %v", loc.Location, err)
	}
	return nil
}

// ensurePinnedCommit makes a pinned full-SHA ref reachable in the cache:
// fetching by SHA works on GitHub and similar hosts; when the server cannot
// serve it, the commit must already be present from the ref fetch.
func ensurePinnedCommit(ctx context.Context, cache string, loc Locator) error {
	ref := loc.Ref
	if _, err := runGitAuth(ctx, loc, "-C", cache, "fetch", "origin", ref); err != nil {
		if _, verifyErr := runGit(ctx, "-C", cache, "rev-parse", ref+"^{commit}"); verifyErr != nil {
			return fmt.Errorf("commit %s is not reachable in the repository", ref)
		}
	}
	if _, err := runGit(ctx, "-C", cache, "rev-parse", ref+"^{commit}"); err != nil {
		return fmt.Errorf("commit %s is not reachable in the repository", ref)
	}
	return nil
}

// gitCacheDir is the per-location cache directory: a content-addressed key
// derived from the normalized location, so one cache serves every check of
// that location and different locations never share a cache.
func gitCacheDir(workDir, location string) string {
	sum := sha256.Sum256([]byte(location))
	return filepath.Join(workDir, "git-cache", hex.EncodeToString(sum[:8]))
}
