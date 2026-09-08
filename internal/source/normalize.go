package source

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ownerRepoRe matches GitHub owner/repo shorthand.
var ownerRepoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Normalize converts user-facing locator fields into the canonical structured
// Locator. An empty kind is inferred: an explicit URL or scp-like location,
// or an owner/repo shorthand that is not an existing directory, is a Git
// Source; everything else is a Local Source. Local locations become
// canonical absolute real paths; Git locations become full Git URLs.
func Normalize(kind Kind, location, ref, subpath string) (Locator, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return Locator{}, fmt.Errorf("location is required")
	}
	if kind == "" {
		kind = inferKind(location)
	}
	switch kind {
	case KindLocal:
		return normalizeLocal(location, ref, subpath)
	case KindGit:
		return normalizeGit(location, ref, subpath)
	default:
		return Locator{}, fmt.Errorf("unsupported Source kind %q (want local or git)", kind)
	}
}

func inferKind(location string) Kind {
	if strings.Contains(location, "://") || strings.HasPrefix(location, "git@") || strings.HasPrefix(location, "github:") {
		return KindGit
	}
	if _, err := os.Stat(location); err == nil {
		return KindLocal
	}
	if ownerRepoRe.MatchString(location) {
		return KindGit
	}
	return KindLocal
}

func normalizeLocal(location, ref, subpath string) (Locator, error) {
	if ref != "" {
		return Locator{}, fmt.Errorf("Local Sources cannot have a ref")
	}
	if strings.TrimSpace(subpath) != "" {
		return Locator{}, fmt.Errorf("Local Sources cannot have a subpath")
	}
	if location == "~" || strings.HasPrefix(location, "~/") || strings.HasPrefix(location, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return Locator{}, err
		}
		location = filepath.Join(home, strings.TrimLeft(strings.TrimPrefix(location, "~"), `/\`))
	}
	abs, err := filepath.Abs(location)
	if err != nil {
		return Locator{}, err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return Locator{Kind: KindLocal, Location: abs}, nil
}

func normalizeGit(location, ref, subpath string) (Locator, error) {
	loc := strings.TrimSuffix(strings.TrimSpace(location), "/")
	if rest, ok := strings.CutPrefix(loc, "github:"); ok {
		// github:owner/repo is the documented GitHub convenience shorthand.
		rest = strings.TrimSuffix(rest, ".git")
		if !ownerRepoRe.MatchString(rest) {
			return Locator{}, fmt.Errorf("invalid github: repository shorthand")
		}
		loc = "https://github.com/" + rest + ".git"
	} else {
		switch {
		case strings.HasPrefix(loc, "https://github.com/") || strings.HasPrefix(loc, "http://github.com/"):
			if !strings.HasSuffix(loc, ".git") {
				loc += ".git"
			}
		case ownerRepoRe.MatchString(loc):
			loc = "https://github.com/" + loc + ".git"
		}
	}
	if !strings.Contains(loc, "://") && !strings.HasPrefix(loc, "git@") {
		return Locator{}, fmt.Errorf("not a Git location: %q", location)
	}
	if err := rejectCredentialUserinfo(loc); err != nil {
		return Locator{}, err
	}
	sub, err := normalizeSubpath(subpath)
	if err != nil {
		return Locator{}, err
	}
	return Locator{Kind: KindGit, Location: loc, Ref: strings.TrimSpace(ref), Subpath: sub}, nil
}

// rejectCredentialUserinfo rejects Git URLs that embed credentials in their
// userinfo. http(s) and git URLs may not carry any userinfo, because
// token-in-URL usage would persist secrets in the locator, database, and
// cache keys; ssh URLs may carry a username (the SSH identity, like
// git@host) but never a password. scp-like git@host:path forms are not URLs
// and remain valid identities. Error messages never echo the location, so a
// rejected secret cannot leak through errors.
func rejectCredentialUserinfo(loc string) error {
	if !strings.Contains(loc, "://") {
		return nil
	}
	u, err := url.Parse(loc)
	if err != nil || u.User == nil {
		return nil
	}
	_, hasPassword := u.User.Password()
	if hasPassword || !strings.EqualFold(u.Scheme, "ssh") {
		return errors.New("Git URLs must not contain embedded credentials")
	}
	return nil
}

// normalizeSubpath canonicalizes a Source subpath: relative, forward-slash
// separated, no leading/trailing separators, and never escaping the Source.
// A leading or trailing slash is tolerated and normalized away, and a
// root-equivalent subpath ('.', './') becomes the empty subpath so the
// unique tuple cannot be bypassed with equivalent spellings.
func normalizeSubpath(subpath string) (string, error) {
	subpath = strings.TrimSpace(subpath)
	subpath = strings.Trim(strings.ReplaceAll(subpath, `\`, "/"), "/")
	if subpath == "" {
		return "", nil
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(subpath)))
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("subpath must not escape the Source: %q", subpath)
	}
	return clean, nil
}

// ResolveLocalSubpath joins a normalized Local Source subpath onto its
// normalized root and verifies, after resolving symlinks, that the result
// still lies inside the root. A symlinked subpath that escapes the Source is
// rejected; the returned directory is the fully resolved real path, so the
// scan never reads through the escaping link.
// ResolveLocalSubpath resolves the observation scope of a Local Source and
// verifies the Source's physical identity. The registered root is a fully
// real path (normalization resolves symlinks at registration), so every
// observation re-evaluates it: if the root itself or any component was
// replaced by a symlink and now resolves elsewhere, the Source's physical
// identity changed and observation is rejected rather than scanning the
// replacement. A missing root is an error (observation reports it as
// unavailable). A non-empty subpath must resolve inside the root without
// escaping through a symlink; the returned directory is the fully resolved
// real path.
func ResolveLocalSubpath(root, subpath string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if realRoot != filepath.Clean(root) {
		return "", fmt.Errorf("Source root was replaced by a symlink or moved; re-register the Source")
	}
	if subpath == "" {
		return realRoot, nil
	}
	target := filepath.Join(root, filepath.FromSlash(subpath))
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(realRoot, realTarget)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("subpath %q escapes the Source through a symlink", subpath)
	}
	return realTarget, nil
}
