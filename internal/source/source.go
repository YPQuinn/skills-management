// Package source owns Source normalization, discovery, and observation for
// Local and Git Sources. It never executes SQL and never writes the Skill
// Store; observations are handed to the application boundary, which persists
// them through internal/state. JSON tags keep CLI --json output on the same
// field names as the REST boundary.
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

// Kind is the underlying Source kind: a local directory or a Git repository.
type Kind string

const (
	KindLocal Kind = "local"
	KindGit   Kind = "git"
)

// Locator is the normalized identity of a Source: kind + location + ref +
// subpath. The tuple is unique, so the same repository may be registered
// more than once only when its ref or subpath differs.
type Locator struct {
	Kind     Kind   `json:"kind"`
	Location string `json:"location"`
	Ref      string `json:"ref,omitempty"`
	Subpath  string `json:"subpath,omitempty"`
}

// Check result values recorded for every Source check. A failed check keeps
// the previous Inventory and only updates check metadata and availability.
const (
	CheckResultOK     = "ok"
	CheckResultFailed = "failed"
)

// Entry is one valid Skill in a Source Inventory, identified by its relative
// Skill directory. Digest is the deterministic content identity of the
// Skill's complete tree (decision 05).
type Entry struct {
	RelativeDir string `json:"relative_dir"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Digest      string `json:"digest,omitempty"`
}

// Issue reports a directory that looks like a Skill but failed validation;
// it is reported and skipped without hiding other valid entries.
type Issue struct {
	RelativeDir string `json:"relative_dir"`
	Reason      string `json:"reason"`
}

// Observation is the outcome of one Source scan: the resolved commit (Git
// only), the aggregate Inventory digest, and the whole Inventory
// replacement, valid entries plus issues.
type Observation struct {
	Commit  string
	Digest  string
	Entries []Entry
	Issues  []Issue
}

// inventoryDigest is the deterministic aggregate digest of one whole Source
// Inventory: the sorted valid entries (relative directory plus tree digest)
// followed by the sorted validation issues (relative directory plus reason).
// It is the Source-level content identity recorded by each check. Caller
// order never matters: the digest hashes sorted defensive copies, so the
// caller's slices are neither mutated nor trusted to be ordered.
func inventoryDigest(entries []Entry, issues []Issue) string {
	es := append([]Entry(nil), entries...)
	sort.Slice(es, func(i, j int) bool {
		if es[i].RelativeDir != es[j].RelativeDir {
			return es[i].RelativeDir < es[j].RelativeDir
		}
		return es[i].Digest < es[j].Digest
	})
	is := append([]Issue(nil), issues...)
	sort.Slice(is, func(i, j int) bool {
		if is[i].RelativeDir != is[j].RelativeDir {
			return is[i].RelativeDir < is[j].RelativeDir
		}
		return is[i].Reason < is[j].Reason
	})
	h := sha256.New()
	for _, e := range es {
		fmt.Fprintf(h, "entry\x00%s\x00%s\n", e.RelativeDir, e.Digest)
	}
	for _, i := range is {
		fmt.Fprintf(h, "issue\x00%s\x00%s\n", i.RelativeDir, i.Reason)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// AddInput is a registration request: user-facing locator fields plus an
// optional display name.
type AddInput struct {
	Kind     Kind
	Location string
	Ref      string
	Subpath  string
	Name     string
}

// Source is one registered Source with its last observation state. The
// embedded Locator flattens into the JSON object.
type Source struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Locator

	Available             bool       `json:"available"`
	LastError             string     `json:"last_error,omitempty"`
	LastCheckStartedAt    *time.Time `json:"last_check_started_at"`
	LastCheckedAt         *time.Time `json:"last_checked_at"`
	LastCheckResult       string     `json:"last_check_result,omitempty"`
	LastSuccessfulCheckAt *time.Time `json:"last_successful_check_at"`
	LastCommit            string     `json:"last_commit,omitempty"`
	LastInventoryDigest   string     `json:"last_inventory_digest,omitempty"`
	Entries               []Entry    `json:"inventory"`
	Issues                []Issue    `json:"issues"`
}

// Summary is one Source list row: identity, availability, and staleness.
type Summary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Locator
	Available             bool       `json:"available"`
	Stale                 bool       `json:"stale"`
	LastError             string     `json:"last_error,omitempty"`
	LastCheckStartedAt    *time.Time `json:"last_check_started_at"`
	LastCheckedAt         *time.Time `json:"last_checked_at"`
	LastCheckResult       string     `json:"last_check_result,omitempty"`
	LastSuccessfulCheckAt *time.Time `json:"last_successful_check_at"`
	LastCommit            string     `json:"last_commit,omitempty"`
	LastInventoryDigest   string     `json:"last_inventory_digest,omitempty"`
	EntryCount            int        `json:"entry_count"`
}

// Observer scans one Source locator and returns its current Inventory.
// workDir is the writable directory for Git caches; Local observation
// ignores it.
type Observer interface {
	Observe(ctx context.Context, loc Locator, workDir string) (Observation, error)
}
