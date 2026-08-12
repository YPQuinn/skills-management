package source

import (
	"context"
	"fmt"
	"os"

	"skillctl/internal/domain"
)

// MaterializeEntry copies the complete tree of one Inventory entry from the
// observed Source into dst, a caller-owned directory outside the Skill
// Store. Local entries are copied from the live directory tree; Git entries
// are materialized from the repository tree at commit, the observed commit
// for the caller's Observation.
//
// The copy is self-contained: symlinks whose targets resolve inside the
// Skill root are dereferenced (their content is copied in place), while a
// broken link, a link escaping the Skill root, a special node (FIFO,
// socket, device), a gitlink, or a canonical Git LFS pointer rejects the
// whole Skill. Relative structure, executable permissions, and empty
// directories are preserved; Source .git metadata of any node type is
// excluded. All filesystem access is rooted (os.Root), so no read can
// escape the Skill root and no write can follow a destination symlink.
//
// allowLarge bypasses the per-Skill guards (10,000 files, 100 MiB total,
// 50 MiB per file); unsafe paths and special nodes can never be overridden.
// The returned string is the canonical content digest of the materialized
// copy. The copy is verified against the observation: the effective
// materialized digest of the source content exactly as observed (internal
// symlinks dereferenced) must equal entry.Digest, so a Source that changed
// after its observation rejects the import instead of installing different
// content.
func MaterializeEntry(ctx context.Context, loc Locator, commit string, entry Entry, workDir, dst string, allowLarge bool) (string, error) {
	if entry.RelativeDir == "" {
		entry.RelativeDir = "."
	}
	if entry.RelativeDir != "." {
		if err := validateRelPath(entry.RelativeDir); err != nil {
			return "", fmt.Errorf("invalid Inventory entry path: %v", err)
		}
	}
	switch loc.Kind {
	case KindLocal:
		return materializeLocal(ctx, loc, entry, dst, allowLarge)
	case KindGit:
		return materializeGit(ctx, loc, commit, entry, workDir, dst, allowLarge)
	default:
		return "", fmt.Errorf("unsupported Source kind %q", loc.Kind)
	}
}

// materializeLocal copies one Local entry with every read rooted at the
// Skill root and every write rooted at dst. The entry's relative directory
// is containment-checked against the resolved Source root before the Skill
// root is opened.
func materializeLocal(ctx context.Context, loc Locator, entry Entry, dst string, allowLarge bool) (string, error) {
	src, err := openLocalEntryRoot(loc, entry.RelativeDir)
	if err != nil {
		return "", err
	}
	defer src.Close()

	// Traverse and count the effective dereferenced tree before creating or
	// copying into the destination. The copy repeats the guard against the
	// opened files, so a concurrent Source change cannot bypass the limits.
	if err := preflightRootTree(ctx, src, ".", &domain.Guard{AllowLarge: allowLarge}, map[fileID]bool{}); err != nil {
		return "", err
	}
	dstRoot, err := openDestinationRoot(dst)
	if err != nil {
		return "", err
	}
	defer dstRoot.Close()

	if err := copyRootTree(ctx, src, ".", dstRoot, &domain.Guard{AllowLarge: allowLarge}, map[fileID]bool{}); err != nil {
		return "", err
	}
	// The materialized tree is symlink-free, so its raw digest is the
	// effective digest; it must equal the observation's Entry.Digest. The
	// final verification runs against the already-open destination handle
	// with the rooted no-follow digest, never an absolute-path re-open.
	digest, err := TreeDigestRoot(ctx, dstRoot)
	if err != nil {
		return "", err
	}
	if digest != entry.Digest {
		return "", fmt.Errorf("Source content changed while it was being imported (expected digest %s, observed %s)", entry.Digest, digest)
	}
	return digest, nil
}

// openLocalEntryRoot anchors both the registered Source root and the
// selected entry with os.Root. A subpath or entry swapped to an escaping
// symlink cannot become the root of an unconfined walk.
func openLocalEntryRoot(loc Locator, relativeDir string) (*os.Root, error) {
	resolvedScope, err := ResolveLocalSubpath(loc.Location, loc.Subpath)
	if err != nil {
		return nil, err
	}
	expected, err := os.Stat(resolvedScope)
	if err != nil {
		return nil, err
	}
	base, err := os.OpenRoot(loc.Location)
	if err != nil {
		return nil, err
	}
	scope := base
	if loc.Subpath != "" {
		scope, err = base.OpenRoot(loc.Subpath)
		base.Close()
		if err != nil {
			return nil, err
		}
	}
	opened, err := scope.Stat(".")
	if err != nil {
		scope.Close()
		return nil, err
	}
	if !os.SameFile(expected, opened) {
		scope.Close()
		return nil, fmt.Errorf("Local Source changed while its root was being opened")
	}
	if relativeDir == "." {
		return scope, nil
	}
	entry, err := scope.OpenRoot(relativeDir)
	scope.Close()
	if err != nil {
		return nil, err
	}
	info, err := entry.Stat(".")
	if err != nil {
		entry.Close()
		return nil, err
	}
	if !info.IsDir() {
		entry.Close()
		return nil, fmt.Errorf("Skill directory %q is not a directory", relativeDir)
	}
	return entry, nil
}
