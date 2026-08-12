package source

import (
	"context"
	"fmt"
	"time"
)

// Local observes Local Sources: canonical absolute paths scanned directly.
// The observation is the inventory plus a content snapshot taken twice with
// a short pause; the inventory is accepted only when both scans agree (see
// observeStable), so a tree that is still changing fails the whole check.
type Local struct {
	// stabilityInterval is the pause between stability scans; zero means
	// the package default.
	stabilityInterval time.Duration
}

// Observe scans the local directory at loc.Location, below loc.Subpath when
// one is set, and returns the stable inventory with per-entry tree digests
// and the aggregate Inventory digest.
func (l Local) Observe(ctx context.Context, loc Locator, _ string) (Observation, error) {
	if loc.Kind != KindLocal {
		return Observation{}, fmt.Errorf("Local observer requires a Local locator")
	}
	root, err := ResolveLocalSubpath(loc.Location, loc.Subpath)
	if err != nil {
		return Observation{}, err
	}
	interval := l.stabilityInterval
	if interval == 0 {
		interval = defaultLocalStabilityInterval
	}
	return observeStable(ctx, interval, func(ctx context.Context) (localScan, error) {
		obs, err := DiscoverCtx(ctx, root)
		if err != nil {
			return localScan{}, err
		}
		snap, err := snapshotRoot(ctx, root)
		if err != nil {
			return localScan{}, err
		}
		// Entry digests are the effective materialized digests: safe
		// internal symlinks are dereferenced, so each digest equals the
		// Store copy import produces. A tree that cannot be materialized
		// (broken or escaping symlink, special node) is an invalid entry
		// reported as an Issue rather than a digest import could never
		// satisfy.
		kept := obs.Entries[:0]
		for _, e := range obs.Entries {
			digest, err := snapshotEffectiveDigest(snap, e.RelativeDir)
			if err != nil {
				obs.Issues = append(obs.Issues, Issue{RelativeDir: e.RelativeDir, Reason: err.Error()})
				continue
			}
			e.Digest = digest
			kept = append(kept, e)
		}
		obs.Entries = kept
		sortObservation(&obs)
		obs.Digest = inventoryDigest(obs.Entries, obs.Issues)
		return localScan{obs: obs, snap: snap}, nil
	})
}
