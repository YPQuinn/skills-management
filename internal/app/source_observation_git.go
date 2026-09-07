package app

import (
	"context"
	"strings"

	"skillctl/internal/source"
)

// reuseGitObservation returns the persisted Source when ls-remote still
// names LastCommit and every Inventory entry already has a complete-tree
// digest. Listing-only registration (empty digests) is never reused.
func (a *App) reuseGitObservation(ctx context.Context, cur *source.Source) (*source.Source, error) {
	if cur.Kind != source.KindGit || !cur.Available || cur.LastCommit == "" || !inventoryComplete(cur.Entries) {
		return nil, nil
	}
	resolved, err := source.ResolveCommit(ctx, cur.Locator)
	if err != nil {
		return nil, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !strings.EqualFold(resolved, cur.LastCommit) {
		return nil, nil
	}
	return cur, nil
}

func inventoryComplete(entries []source.Entry) bool {
	if len(entries) == 0 {
		return true
	}
	for _, e := range entries {
		if e.Digest == "" {
			return false
		}
	}
	return true
}
