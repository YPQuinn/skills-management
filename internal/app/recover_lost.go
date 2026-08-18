package app

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"skillctl/internal/domain"
	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// StoreRecoveryResult is the report of rebuilding state.db from live
// Store directories after the database was lost.
type StoreRecoveryResult struct {
	Recovered         []RecoveredSkill    `json:"recovered"`
	Skipped           []SkippedStoreEntry `json:"skipped"`
	PreservedInternal []string            `json:"preserved_internal"`
	Unrecoverable     []string            `json:"unrecoverable"`
}

// RecoveredSkill is one top-level Store directory adopted as a newly
// identified unbound Skill.
type RecoveredSkill struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	StoreDigest string `json:"store_digest"`
}

// SkippedStoreEntry is a top-level Store name that was left untouched
// because it is not a valid Skill directory.
type SkippedStoreEntry struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// unrecoverableAfterStateLoss are the relationship kinds that live only
// in SQLite and cannot be reconstructed from the Store tree.
var unrecoverableAfterStateLoss = []string{
	"sources", "groups", "assignments", "targets", "managed_links",
}

// RecoverLostStore adopts valid top-level Store directories as unbound
// Skills. It never rewrites Store content, never guesses leftover
// Baseline/snapshot ownership, and never recreates Sources, Groups,
// Assignments, Targets, or Managed Links.
func (a *App) RecoverLostStore(ctx context.Context) (*StoreRecoveryResult, error) {
	held, err := a.acquireStoreLock()
	if err != nil {
		return nil, err
	}
	defer held.Unlock()

	root, err := os.OpenRoot(a.StorePath)
	if err != nil {
		return nil, Errorf(CodeInternal, "opening the Skill Store: %v", err)
	}
	defer root.Close()

	if err := a.reserveLostStoreIDs(root); err != nil {
		return nil, err
	}
	recovered, skipped, err := a.adoptLostStoreSkills(ctx, root)
	if err != nil {
		return nil, err
	}
	return &StoreRecoveryResult{
		Recovered:         recovered,
		Skipped:           skipped,
		PreservedInternal: listPreservedInternal(root),
		Unrecoverable:     append([]string(nil), unrecoverableAfterStateLoss...),
	}, nil
}

func (a *App) reserveLostStoreIDs(root *os.Root) error {
	internal, err := skillstore.OpenPinnedChild(root, ".skillctl")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		// A non-directory or symlink .skillctl is left untouched; there
		// are no leftover numeric children to reserve past.
		return nil
	}
	defer internal.Close()
	skillAtLeast := maxNumericChild(internal, "baselines")
	if n := maxNumericChild(internal, "previous"); n > skillAtLeast {
		skillAtLeast = n
	}
	opAtLeast := maxNumericChild(internal, "staging")
	if n := maxNumericChild(internal, "recovery"); n > opAtLeast {
		opAtLeast = n
	}
	if err := state.ReserveAutoIncrement(a.db, "skills", skillAtLeast); err != nil {
		return Errorf(CodeInternal, "reserving Skill ids: %v", err)
	}
	if err := state.ReserveAutoIncrement(a.db, "store_operations", opAtLeast); err != nil {
		return Errorf(CodeInternal, "reserving operation ids: %v", err)
	}
	return nil
}

func (a *App) adoptLostStoreSkills(ctx context.Context, root *os.Root) ([]RecoveredSkill, []SkippedStoreEntry, error) {
	names, err := listRootNames(root)
	if err != nil {
		return nil, nil, Errorf(CodeInternal, "reading the Skill Store: %v", err)
	}
	recovered := []RecoveredSkill{}
	skipped := []SkippedStoreEntry{}
	now := time.Now().UTC()
	for _, name := range names {
		if name == ".skillctl" {
			continue
		}
		if a.recoverNameHook != nil {
			a.recoverNameHook(name)
		}
		child, err := skillstore.OpenPinnedChild(root, name)
		if err != nil {
			skipped = append(skipped, SkippedStoreEntry{Name: name, Reason: skipReason(err)})
			continue
		}
		if err := domain.ValidateSlug(name); err != nil {
			child.Close()
			skipped = append(skipped, SkippedStoreEntry{Name: name, Reason: err.Error()})
			continue
		}
		skillName, desc, err := source.ReadSkillRoot(child)
		if err != nil {
			child.Close()
			skipped = append(skipped, SkippedStoreEntry{Name: name, Reason: err.Error()})
			continue
		}
		digest, err := source.TreeDigestRoot(ctx, child)
		child.Close()
		if err != nil {
			return nil, nil, Errorf(CodeInternal, "digesting Store Skill %q: %v", name, err)
		}
		id, err := state.InsertUnboundSkill(a.db, state.Skill{
			Slug: name, Name: skillName, Description: desc,
			StoreDigest: digest, BaselineDigest: "",
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return nil, nil, Errorf(CodeInternal, "recording recovered Skill %q: %v", name, err)
		}
		recovered = append(recovered, RecoveredSkill{
			ID: id, Slug: name, Name: skillName, Description: desc, StoreDigest: digest,
		})
	}
	sort.Slice(recovered, func(i, j int) bool { return recovered[i].Slug < recovered[j].Slug })
	sort.Slice(skipped, func(i, j int) bool { return skipped[i].Name < skipped[j].Name })
	return recovered, skipped, nil
}

func listPreservedInternal(root *os.Root) []string {
	out := []string{}
	internal, err := skillstore.OpenPinnedChild(root, ".skillctl")
	if err != nil {
		return out
	}
	defer internal.Close()
	for _, kind := range []string{"baselines", "previous", "staging", "recovery"} {
		child, err := skillstore.OpenPinnedChild(internal, kind)
		if err != nil {
			continue
		}
		names, err := listRootNames(child)
		child.Close()
		if err != nil {
			continue
		}
		for _, name := range names {
			out = append(out, filepath.ToSlash(filepath.Join(".skillctl", kind, name)))
		}
	}
	sort.Strings(out)
	return out
}

func maxNumericChild(parent *os.Root, name string) int64 {
	child, err := skillstore.OpenPinnedChild(parent, name)
	if err != nil {
		return 0
	}
	defer child.Close()
	names, err := listRootNames(child)
	if err != nil {
		return 0
	}
	var max int64
	for _, n := range names {
		v, err := strconv.ParseInt(n, 10, 64)
		if err == nil && v > max {
			max = v
		}
	}
	return max
}

func listRootNames(root *os.Root) ([]string, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func skipReason(err error) string {
	if err == nil {
		return "not a Skill directory"
	}
	return err.Error()
}
