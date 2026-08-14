package source

import (
	"context"
	"fmt"
	"strings"

	"skillctl/internal/domain"
)

// materializeGit materializes one Git entry from the repository tree at the
// observed commit. The tree listing carries blob sizes so the per-Skill
// guards are enforced before any blob is fetched or buffered; the
// observation-form effective digest (computed by the same function the
// observer uses) must equal entry.Digest, so content that changed after its
// observation — or a commit that is not the observed one — rejects the
// import instead of installing different content. Every destination path is
// validated canonical and written rooted, so a crafted tree cannot escape
// the output directory or follow a planted symlink.
func materializeGit(ctx context.Context, loc Locator, commit string, entry Entry, workDir, dst string, allowLarge bool) (string, error) {
	if !isFullSHA(commit) {
		return "", fmt.Errorf("Git materialization requires the full observed commit")
	}
	cache := gitCacheDir(workDir, loc.Location)
	if err := ensureCache(ctx, loc, cache); err != nil {
		return "", err
	}
	if isFullSHA(loc.Ref) {
		if err := ensurePinnedCommit(ctx, cache, loc); err != nil {
			return "", err
		}
	}
	args := []string{"-C", cache, "ls-tree", "-z", "-r", "-l", commit}
	if loc.Subpath != "" {
		args = append(args, "--", loc.Subpath)
	}
	out, err := runGit(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("reading tree %s: %v", commit, err)
	}
	rows, err := parseTreeRecords(out)
	if err != nil {
		return "", err
	}
	for i := range rows {
		rows[i].rel, err = gitRelativePath(rows[i].path, loc.Subpath)
		if err != nil {
			return "", err
		}
	}
	dir := ""
	if entry.RelativeDir != "." {
		dir = entry.RelativeDir
	}
	skillRows := skillRows(rows, dir)
	if len(skillRows) == 0 {
		return "", fmt.Errorf("entry %q does not exist in the repository at %s", entry.RelativeDir, commit)
	}
	for _, l := range skillRows {
		rel := l.rel
		if dir != "" {
			rel = strings.TrimPrefix(rel, dir+"/")
		}
		if err := validateRelPath(rel); err != nil {
			return "", err
		}
	}
	// Fetch only the small symlink blobs needed to count the exact
	// dereferenced output. Oversized regular content is rejected from
	// ls-tree sizes before those blobs are fetched or buffered.
	if !allowLarge {
		linkBlobs, err := fetchTreeBlobs(ctx, cache, gitSymlinkRows(skillRows), loc)
		if err != nil {
			return "", err
		}
		if err := preflightGitMaterializedGuards(skillRows, dir, linkBlobs); err != nil {
			return "", err
		}
	}
	blobs, err := fetchTreeBlobs(ctx, cache, skillRows, loc)
	if err != nil {
		return "", err
	}
	digest, err := skillTreeEffectiveDigest(skillRows, dir, blobs)
	if err != nil {
		return "", err
	}
	if digest != entry.Digest {
		return "", fmt.Errorf("%w (expected digest %s, observed %s)", ErrContentChanged, entry.Digest, digest)
	}
	dstRoot, err := openDestinationRoot(dst)
	if err != nil {
		return "", err
	}
	defer dstRoot.Close()
	if err := writeGitTree(ctx, skillRows, dir, blobs, dstRoot, &domain.Guard{AllowLarge: allowLarge}); err != nil {
		return "", err
	}
	// The written tree is symlink-free, so its raw digest is the effective
	// digest and must equal the observation's Entry.Digest. The final
	// verification runs against the already-open destination handle with
	// the rooted no-follow digest, never an absolute-path re-open.
	written, err := TreeDigestRoot(ctx, dstRoot)
	if err != nil {
		return "", err
	}
	if written != entry.Digest {
		return "", fmt.Errorf("materialized content digest %s does not match the observed digest %s", written, entry.Digest)
	}
	return written, nil
}
