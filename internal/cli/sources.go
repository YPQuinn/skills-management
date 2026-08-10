package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"skillctl/internal/bootstrap"
	"skillctl/internal/source"
)

// NewSourceCmd builds the noun-first `skillctl source` command group.
func NewSourceCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage upstream Sources",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		NewSourceAddCmd(bm),
		NewSourceListCmd(bm),
		NewSourceShowCmd(bm),
		NewSourceCheckCmd(bm),
	)
	return cmd
}

func NewSourceAddCmd(bm *bootstrap.Manager) *cobra.Command {
	var kind, name, ref, subpath string
	cmd := &cobra.Command{
		Use:   "add <location>",
		Short: "Register an upstream Source and scan its Inventory",
		Long: `Register an upstream Source and scan its Inventory. The Source is
reached and scanned during registration but nothing is imported; import is a
separate explicit step.

A Local Source is a directory path (absolute, or relative to the current
directory). A Git Source is an HTTPS/SSH URL, or GitHub owner/repo shorthand.
Omitted --kind is inferred: URLs and owner/repo shorthand are Git, existing
directories are Local.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			created, err := a.AddSource(cmd.Context(), source.AddInput{
				Kind:     source.Kind(kind),
				Location: args[0],
				Ref:      ref,
				Subpath:  subpath,
				Name:     name,
			})
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, newSourceJSONView(created))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Registered Source %q (%s) with %d Skill(s)\n",
				created.Name, created.Location, len(created.Entries))
			if len(created.Issues) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%d invalid %s skipped\n", len(created.Issues), entryWord(len(created.Issues)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "Source kind: local or git (inferred when omitted)")
	cmd.Flags().StringVar(&name, "name", "", "display name (defaults to the location base name)")
	cmd.Flags().StringVar(&ref, "ref", "", "Git branch, tag, or pinned commit SHA (default branch when omitted)")
	cmd.Flags().StringVar(&subpath, "subpath", "", "directory below the Source root to scan")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewSourceListCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered Sources",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			items, err := a.ListSources()
			if err != nil {
				return err
			}
			if jsonRequested {
				if items == nil {
					items = []source.Summary{}
				}
				return printJSON(cmd, map[string]any{"items": items, "total": len(items)})
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No Sources registered.")
				return nil
			}
			for _, s := range items {
				state := "available"
				if !s.Available {
					state = "unavailable"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\t%d Skill(s)\t%s\n",
					s.ID, s.Name, s.Kind, s.Location, s.EntryCount, state)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewSourceShowCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show one Source and its Inventory",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSourceArg(args[0])
			if err != nil {
				return err
			}
			s, err := a.ShowSource(id)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, newSourceJSONView(s))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Source %d: %s\n", s.ID, s.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Kind: %s\nLocation: %s\n", s.Kind, s.Location)
			if s.Ref != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Ref: %s\n", s.Ref)
			}
			if s.Subpath != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Subpath: %s\n", s.Subpath)
			}
			if s.LastCommit != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Commit: %s\n", s.LastCommit)
			}
			if !s.Available {
				fmt.Fprintf(cmd.OutOrStdout(), "Status: unavailable (%s)\n", s.LastError)
				fmt.Fprintln(cmd.OutOrStdout(), "Inventory is stale; the last successful check was",
					formatHumanTime(s.LastSuccessfulCheckAt))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Status: available")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Last checked: %s\n", formatHumanTime(s.LastCheckedAt))
			if s.LastCheckResult != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Check result: %s\n", s.LastCheckResult)
			}
			if s.LastInventoryDigest != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Inventory digest: %s\n", s.LastInventoryDigest)
			}
			for _, e := range s.Entries {
				fmt.Fprintf(cmd.OutOrStdout(), "Skill: %s\t%s\t%s\n", e.RelativeDir, e.Name, e.Description)
			}
			for _, i := range s.Issues {
				fmt.Fprintf(cmd.OutOrStdout(), "Invalid: %s\t%s\n", i.RelativeDir, i.Reason)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewSourceCheckCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <name-or-id>",
		Short: "Re-scan one Source and replace its Inventory",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSourceArg(args[0])
			if err != nil {
				return err
			}
			s, err := a.CheckSource(cmd.Context(), id)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, newSourceJSONView(s))
			}
			if !s.Available {
				// The check itself succeeded; it recorded unavailability.
				fmt.Fprintf(cmd.OutOrStdout(), "Source %q is unavailable: %s\n", s.Name, s.LastError)
				fmt.Fprintln(cmd.OutOrStdout(), "The previous Inventory is retained but stale.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Source %q is available with %d Skill(s)\n", s.Name, len(s.Entries))
			if s.LastCommit != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Commit: %s\n", s.LastCommit)
			}
			if len(s.Issues) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%d invalid %s skipped\n", len(s.Issues), entryWord(len(s.Issues)))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// sourceJSONView is the CLI --json detail shape: the Source fields plus the
// derived list-style fields, matching the REST detail resource. Empty
// collections encode as [] rather than null.
type sourceJSONView struct {
	source.Source
	Stale      bool `json:"stale"`
	EntryCount int  `json:"entry_count"`
}

func newSourceJSONView(s *source.Source) sourceJSONView {
	v := sourceJSONView{Source: *s, Stale: !s.Available, EntryCount: len(s.Entries)}
	if v.Entries == nil {
		v.Entries = []source.Entry{}
	}
	if v.Issues == nil {
		v.Issues = []source.Issue{}
	}
	return v
}

// printJSON writes exactly one JSON value to the command stdout.
func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func formatHumanTime(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func entryWord(n int) string {
	if n == 1 {
		return "entry"
	}
	return "entries"
}
