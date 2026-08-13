package app

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"skillctl/internal/state"
	"skillctl/internal/target"
)

// Target is one app-facing Target row: the fixed physical path identity
// plus the explanatory creation metadata.
type Target struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Adapter     string    `json:"adapter"`
	Scope       string    `json:"scope"`
	ProjectRoot string    `json:"project_root,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TargetView is the Target detail: identity, compatible adapters derived
// from the current table, its direct Skill and Group Assignments, and the
// expanded desired Skill set with explanations.
type TargetView struct {
	Target
	CompatibleAdapters []string         `json:"compatible_adapters,omitempty"`
	DirectSkills       []AssignmentView `json:"direct_skills"`
	Groups             []AssignmentView `json:"groups"`
	DesiredSkills      []DesiredSkill   `json:"desired_skills"`
}

// TargetInput is one Target registration request: either a built-in
// adapter with a user or project scope, or a custom container path. The
// two forms are exclusive.
type TargetInput struct {
	Name        string
	Adapter     string
	Scope       string
	ProjectRoot string
	Path        string
}

// resolveOptions builds the resolution environment from the process state:
// the home directory and the real environment lookup. Adapter overrides
// are read only during Target creation or detection (decision 04/08).
func resolveOptions() (target.ResolveOptions, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return target.ResolveOptions{}, Errorf(CodeInternal, "cannot determine the home directory: %v", err)
	}
	return target.ResolveOptions{Home: home}, nil
}

// validateTargetName checks an operator-facing Target name with the same
// boundary rules as a Source or Group name.
func validateTargetName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("Target name must not be empty")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("Target name must not contain %q", "/")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("Target name must not be %q", name)
	}
	return nil
}

// defaultTargetName derives a display name from the creation metadata when
// the operator did not choose one: the adapter and scope for built-in
// Targets, and the container's base name for custom Targets.
func defaultTargetName(rt target.ResolvedTarget) string {
	base := ""
	switch rt.Scope {
	case target.ScopeUser:
		return rt.Adapter + "-user"
	case target.ScopeProject:
		base = filepath.Base(rt.ProjectRoot)
	default:
		base = filepath.Base(rt.Path)
	}
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "target"
	}
	return rt.Adapter + "-" + base
}

// RegisterTarget registers one Target. Registration is read-only: it
// resolves and stores the normalized absolute physical path and its
// creation metadata, performs no filesystem mutation, and returns the
// existing Target when the resolved path is already registered.
func (a *App) RegisterTarget(in TargetInput) (*TargetView, error) {
	custom := in.Path != ""
	switch {
	case !custom && in.Adapter == "":
		return nil, Errorf(CodeInvalidArgument, "either a custom path or a Target Adapter is required")
	case custom && (in.Adapter != "" || in.Scope != "" || in.ProjectRoot != ""):
		return nil, Errorf(CodeInvalidArgument, "a custom path cannot be combined with an adapter, scope, or project root")
	}

	opts, err := resolveOptions()
	if err != nil {
		return nil, err
	}
	var rt target.ResolvedTarget
	if custom {
		rt, err = target.ResolveCustom(in.Path, opts)
	} else {
		adapter, ok := target.ByKey(in.Adapter)
		if !ok {
			return nil, Errorf(CodeInvalidArgument, "unknown Target Adapter %q", in.Adapter)
		}
		switch target.Scope(in.Scope) {
		case target.ScopeUser:
			rt, err = target.ResolveUser(adapter, opts)
		case target.ScopeProject:
			rt, err = target.ResolveProject(adapter, in.ProjectRoot, opts)
		default:
			return nil, Errorf(CodeInvalidArgument, "Target scope must be %q or %q", target.ScopeUser, target.ScopeProject)
		}
	}
	if err != nil {
		return nil, Errorf(CodeInvalidArgument, "%v", err)
	}
	if err := target.CheckStoreDisjoint(rt.Path, a.StorePath, opts); err != nil {
		return nil, Errorf(CodeInvalidArgument, "%v", err)
	}

	if existing, err := state.GetTargetByPath(a.db, rt.Path); err == nil {
		return a.ShowTarget(existing.ID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeInternal, "reading Target by path: %v", err)
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = defaultTargetName(rt)
	}
	if err := validateTargetName(name); err != nil {
		return nil, Errorf(CodeInvalidArgument, "%v", err)
	}
	now := time.Now().UTC()
	id, err := state.InsertTarget(a.db, state.Target{
		Name: name, Path: rt.Path, Adapter: rt.Adapter, Scope: string(rt.Scope),
		ProjectRoot: rt.ProjectRoot, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		if state.IsUniqueViolation(err) {
			if strings.Contains(err.Error(), "targets.path") {
				if existing, err2 := state.GetTargetByPath(a.db, rt.Path); err2 == nil {
					return a.ShowTarget(existing.ID)
				}
			}
			return nil, Errorf(CodeConflict, "a Target named %q is already registered", name)
		}
		return nil, Errorf(CodeInternal, "saving Target: %v", err)
	}
	return a.ShowTarget(id)
}

// ListTargets returns all registered Targets in name order.
func (a *App) ListTargets() ([]Target, error) {
	rows, err := state.ListTargets(a.db)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Targets: %v", err)
	}
	out := make([]Target, 0, len(rows))
	for _, t := range rows {
		out = append(out, Target{
			ID: t.ID, Name: t.Name, Path: t.Path, Adapter: t.Adapter,
			Scope: t.Scope, ProjectRoot: t.ProjectRoot,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		})
	}
	return out, nil
}

// ShowTarget returns one Target with its assignments and desired set.
func (a *App) ShowTarget(id int64) (*TargetView, error) {
	t, err := state.GetTargetByID(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Target %d not found", id)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d: %v", id, err)
	}
	view := &TargetView{
		Target: Target{
			ID: t.ID, Name: t.Name, Path: t.Path, Adapter: t.Adapter,
			Scope: t.Scope, ProjectRoot: t.ProjectRoot,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		},
		DirectSkills: []AssignmentView{},
		Groups:       []AssignmentView{},
	}
	if t.Scope != string(target.ScopeCustom) {
		opts, err := resolveOptions()
		if err != nil {
			return nil, err
		}
		view.CompatibleAdapters = target.CompatibleAdapters(t.Path, target.Scope(t.Scope), t.ProjectRoot, opts)
	}
	details, err := state.ListTargetAssignmentDetails(a.db, id)
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Target %d Assignments: %v", id, err)
	}
	for _, d := range details {
		if d.Kind == "skill" {
			view.DirectSkills = append(view.DirectSkills, assignmentView(d))
		} else {
			view.Groups = append(view.Groups, assignmentView(d))
		}
	}
	view.DesiredSkills, err = a.desiredSkills(id)
	if err != nil {
		return nil, err
	}
	return view, nil
}

// ResolveTargetArg maps a CLI argument to a Target id: numeric arguments
// are the id itself, anything else is the unique name.
func (a *App) ResolveTargetArg(arg string) (int64, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		return id, nil
	}
	id, err := state.TargetIDByName(a.db, arg)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, Errorf(CodeNotFound, "Target %q not found", arg)
	}
	if err != nil {
		return 0, Errorf(CodeInternal, "resolving Target %q: %v", arg, err)
	}
	return id, nil
}
