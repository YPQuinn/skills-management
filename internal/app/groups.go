package app

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"skillctl/internal/state"
)

// Group is one app-facing Group row.
type Group struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GroupSummary is one Group list row with its membership count.
type GroupSummary struct {
	Group
	MemberCount int `json:"member_count"`
}

// SkillRef is the minimal identity of one Skill for relationship views.
type SkillRef struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// TargetRef is the minimal identity of one Target for relationship views.
type TargetRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// GroupRef is the minimal identity of one Group for relationship views.
type GroupRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// GroupView is the Group detail: membership plus the Targets that assign
// the Group, with the same identity fields as a Target list row.
type GroupView struct {
	Group
	Members []SkillRef `json:"members"`
	Targets []Target   `json:"targets"`
}

// validateGroupName checks an operator-facing Group name with the same
// boundary rules as a Source name. Uniqueness is enforced by the state
// schema.
func validateGroupName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("Group name must not be empty")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("Group name must not contain %q", "/")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("Group name must not be %q", name)
	}
	return nil
}

// CreateGroup registers one named Group. Surrounding whitespace is
// trimmed before validation and persistence, so the stored name is the
// operator-facing value rather than the raw input.
func (a *App) CreateGroup(name string) (*Group, error) {
	name = strings.TrimSpace(name)
	if err := validateGroupName(name); err != nil {
		return nil, Errorf(CodeInvalidArgument, "%v", err)
	}
	now := time.Now().UTC()
	id, err := state.InsertGroup(a.db, state.Group{Name: name, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		if state.IsUniqueViolation(err) {
			return nil, Errorf(CodeConflict, "a Group named %q is already registered", name)
		}
		return nil, Errorf(CodeInternal, "saving Group: %v", err)
	}
	return &Group{ID: id, Name: name, CreatedAt: now, UpdatedAt: now}, nil
}

// ListGroups returns all Groups in name order with member counts.
func (a *App) ListGroups() ([]GroupSummary, error) {
	rows, err := state.ListGroupSummaries(a.db)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Groups: %v", err)
	}
	out := make([]GroupSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, GroupSummary{
			Group: Group{
				ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			},
			MemberCount: r.MemberCount,
		})
	}
	return out, nil
}

// ShowGroup returns one Group with its members and assigning Targets.
func (a *App) ShowGroup(id int64) (*GroupView, error) {
	g, err := state.GetGroupByID(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Group %d not found", id)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Group %d: %v", id, err)
	}
	view := &GroupView{
		Group:   Group{ID: g.ID, Name: g.Name, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt},
		Members: []SkillRef{},
		Targets: []Target{},
	}
	members, err := state.ListGroupMemberSkills(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Group %d members: %v", id, err)
	}
	for _, m := range members {
		view.Members = append(view.Members, SkillRef{ID: m.ID, Slug: m.Slug, Name: m.Name})
	}
	targets, err := state.ListGroupTargets(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Group %d Targets: %v", id, err)
	}
	for _, t := range targets {
		item := targetFromState(&t)
		item.LastResult = t.LastDistResult
		item.Stale = t.LastInspectedStale
		view.Targets = append(view.Targets, item)
	}
	return view, nil
}

// ResolveGroupArg maps a CLI argument to a Group id: numeric arguments are
// the id itself, anything else is the unique name.
func (a *App) ResolveGroupArg(arg string) (int64, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		return id, nil
	}
	g, err := state.GetGroupByName(a.db, arg)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, Errorf(CodeNotFound, "Group %q not found", arg)
	}
	if err != nil {
		return 0, Errorf(CodeInternal, "resolving Group %q: %v", arg, err)
	}
	return g.ID, nil
}

// AddGroupSkills adds every given Skill to the Group; adding a member that
// is already present is a no-op. It returns the refreshed Group view.
func (a *App) AddGroupSkills(groupID int64, skillIDs []int64) (*GroupView, error) {
	if _, err := state.GetGroupByID(a.db, groupID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Group %d not found", groupID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Group %d: %v", groupID, err)
	}
	for _, id := range skillIDs {
		if _, err := state.GetSkillDetailByID(a.db, id); errors.Is(err, sql.ErrNoRows) {
			return nil, Errorf(CodeNotFound, "Skill %d not found", id)
		} else if err != nil {
			return nil, Errorf(CodeInternal, "reading Skill %d: %v", id, err)
		}
	}
	if err := state.AddGroupMembers(a.db, groupID, skillIDs, time.Now().UTC()); err != nil {
		return nil, Errorf(CodeInternal, "adding Group members: %v", err)
	}
	return a.ShowGroup(groupID)
}

// RemoveGroupSkills removes every given Skill from the Group; removing a
// non-member is a no-op. It returns the refreshed Group view.
func (a *App) RemoveGroupSkills(groupID int64, skillIDs []int64) (*GroupView, error) {
	if _, err := state.GetGroupByID(a.db, groupID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Group %d not found", groupID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Group %d: %v", groupID, err)
	}
	if err := state.RemoveGroupMembers(a.db, groupID, skillIDs); err != nil {
		return nil, Errorf(CodeInternal, "removing Group members: %v", err)
	}
	return a.ShowGroup(groupID)
}
