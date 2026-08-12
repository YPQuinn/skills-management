package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"skillctl/internal/lock"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// Additional stable error codes for resource and Source operations.
const (
	CodeNotFound          = "not_found"
	CodeConflict          = "conflict"
	CodeSourceUnavailable = "source_unavailable"
)

// dispatchObserver routes a Locator to its Local or Git observer.
type dispatchObserver struct {
	local source.Local
	git   source.Git
}

func (d dispatchObserver) Observe(ctx context.Context, loc source.Locator, workDir string) (source.Observation, error) {
	switch loc.Kind {
	case source.KindLocal:
		return d.local.Observe(ctx, loc, workDir)
	case source.KindGit:
		return d.git.Observe(ctx, loc, workDir)
	}
	return source.Observation{}, fmt.Errorf("unsupported Source kind %q", loc.Kind)
}

// AddSource registers one Source: the locator is normalized, the Source is
// reached and scanned, and the whole Inventory is persisted. A Source that
// cannot be reached and scanned is not saved. Registration never imports.
// A cancelled context before persistence returns without saving anything.
func (a *App) AddSource(ctx context.Context, in source.AddInput) (*source.Source, error) {
	loc, err := source.Normalize(in.Kind, in.Location, in.Ref, in.Subpath)
	if err != nil {
		return nil, Errorf(CodeInvalidArgument, "%v", err)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = defaultSourceName(loc)
	}
	if err := validateSourceName(name); err != nil {
		return nil, Errorf(CodeInvalidArgument, "%v", err)
	}
	if err := a.checkLocalStoreOverlap(loc); err != nil {
		return nil, err
	}
	started := time.Now().UTC()
	obs, err := a.observer.Observe(ctx, loc, a.workDir())
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		if errors.Is(err, lock.ErrLocked) {
			return nil, Errorf(CodeLocked, "another skillctl process is already checking this Source")
		}
		return nil, Errorf(CodeSourceUnavailable, "Source %s is not reachable: %v", loc.Location, err)
	}
	now := time.Now().UTC()
	s := source.Source{
		Name:                  name,
		Locator:               loc,
		CreatedAt:             now,
		UpdatedAt:             now,
		Available:             true,
		LastCheckStartedAt:    &started,
		LastCheckedAt:         &now,
		LastCheckResult:       source.CheckResultOK,
		LastSuccessfulCheckAt: &now,
		LastCommit:            obs.Commit,
		LastInventoryDigest:   obs.Digest,
		Entries:               obs.Entries,
		Issues:                obs.Issues,
	}
	id, err := state.InsertSource(a.db, s)
	if err != nil {
		if state.IsUniqueViolation(err) {
			// The same locator tuple and the same name are distinct
			// conflicts; classify by re-reading what actually collides.
			if _, err2 := state.SourceIDByTuple(a.db, loc); err2 == nil {
				return nil, Errorf(CodeConflict, "a Source with this location, ref, and subpath is already registered")
			}
			if exists, err2 := state.SourceNameExists(a.db, name); err2 == nil && exists {
				return nil, Errorf(CodeConflict, "a Source named %q is already registered", name)
			}
		}
		return nil, Errorf(CodeInternal, "saving Source: %v", err)
	}
	s.ID = id
	return &s, nil
}

// CheckSource re-observes one Source. A successful check replaces the whole
// Inventory and records the resolved commit, the aggregate Inventory digest,
// and check metadata; a failed check keeps the previous Inventory, commit,
// and digest and records only availability plus latest check metadata. The
// use case itself succeeds either way. The per-Source cross-process lock is
// acquired before the Source row is read, so a check can never apply or
// respond from stale pre-lock state written by a concurrent check; lock
// contention is CodeLocked and mutates nothing. A cancelled context before
// persistence returns without recording anything.
func (a *App) CheckSource(ctx context.Context, id int64) (*source.Source, error) {
	return a.observeSource(ctx, id)
}

// ListSources returns all registered Sources as list rows.
func (a *App) ListSources() ([]source.Summary, error) {
	out, err := state.ListSources(a.db)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Sources: %v", err)
	}
	return out, nil
}

// ShowSource returns one Source with its Inventory and issues.
func (a *App) ShowSource(id int64) (*source.Source, error) {
	s, err := state.GetSource(a.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, Errorf(CodeNotFound, "Source %d not found", id)
		}
		return nil, Errorf(CodeInternal, "reading Source %d: %v", id, err)
	}
	return s, nil
}

// ResolveSourceArg maps a CLI argument to a Source id: a numeric argument
// is the id itself, anything else is an operator-facing name. Names are
// unique, so the resolution is unambiguous.
func (a *App) ResolveSourceArg(arg string) (int64, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		return id, nil
	}
	id, err := state.SourceIDByName(a.db, arg)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, Errorf(CodeNotFound, "Source %q not found", arg)
	}
	if err != nil {
		return 0, Errorf(CodeInternal, "resolving Source %q: %v", arg, err)
	}
	return id, nil
}

// workDir is the writable directory for Git caches and their locks, beside
// state.db; gitCacheDir places each cache at <workDir>/git-cache/<key>.
func (a *App) workDir() string {
	return filepath.Dir(a.StateDBPath)
}

// sourceLockPath is the cross-process lock serializing checks of one Source.
// It lives beside state.db with the other lock files.
func (a *App) sourceLockPath(id int64) (string, error) {
	dir := filepath.Join(filepath.Dir(a.StateDBPath), "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("source-%d.lock", id)), nil
}

// checkLocalStoreOverlap rejects a Local Source whose resolved scan scope
// overlaps the Skill Store in either direction after resolving real paths,
// so the Store can never be read as upstream Source content. Resolving the
// scope also verifies the Source root's physical identity (a symlink
// replacement is rejected). When the Source directory is simply missing,
// the check is skipped and observation reports the unavailability instead.
func (a *App) checkLocalStoreOverlap(loc source.Locator) error {
	if loc.Kind != source.KindLocal {
		return nil
	}
	sourceRoot, err := source.ResolveLocalSubpath(loc.Location, loc.Subpath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return Errorf(CodeInvalidArgument, "%v", err)
	}
	store, err := filepath.EvalSymlinks(a.StorePath)
	if err != nil {
		return nil
	}
	if pathsOverlap(sourceRoot, store) {
		return Errorf(CodeInvalidArgument, "a Local Source and the Skill Store must not overlap")
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	return pathWithin(a, b) || pathWithin(b, a)
}

func pathWithin(inner, outer string) bool {
	rel, err := filepath.Rel(outer, inner)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
