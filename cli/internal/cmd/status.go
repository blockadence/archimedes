package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes/cli/internal/status"
)

func newStatusCmd() *cobra.Command {
	var root string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status [slug]",
		Short: "Live PR/branch status across every worktree this instance has spawned",
		Long: `Reads every work/<slug>/status.md this instance has recorded and looks up
each row's live PR state via "gh pr list". Pass a slug to limit the report
to one unit of work. Warns when more worktree streams are open than
ARCHIMEDES_MAX_STREAMS (default 3) allows.

Also flags any stacked unit of work whose base branch has since merged, so
a dependent branch is reported as needing a rebase rather than quietly
going stale. The flag clears once the branch has been rebased.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var slugFilter string
			if len(args) == 1 {
				slugFilter = args[0]
			}
			return runStatus(c.OutOrStdout(), root, slugFilter, jsonOutput, status.Sources{
				PR:     status.GHLookup,
				Refs:   status.LocalRefs{},
				Merged: status.GHMerged,
			})
		},
	}

	cmd.Flags().StringVar(&root, "root", ".", "instance root containing repos.yaml and work/")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print a machine-readable JSON report instead of the human-readable table")

	return cmd
}

// runStatus builds and prints the report. src carries the report's PR,
// ref, and merged-state sources; its Repos is filled in here, since only
// this layer knows the instance root the manifest resolves against.
func runStatus(w io.Writer, root, slugFilter string, jsonOutput bool, src status.Sources) error {
	m, err := loadManifest(root)
	if err != nil {
		return err
	}
	src.Repos = func(name string) (status.RepoRef, error) {
		r, err := m.Resolve(root, name)
		if err != nil {
			return status.RepoRef{}, err
		}
		return status.RepoRef{Path: r.Path, BaseBranch: r.BaseBranch}, nil
	}

	entries, err := status.Discover(filepath.Join(root, "work"), slugFilter)
	if err != nil {
		return fmt.Errorf("discovering status files: %w", err)
	}

	report := status.BuildReport(entries, src, status.ParseGuardrailMax(os.Getenv("ARCHIMEDES_MAX_STREAMS")))

	if jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	_, err = fmt.Fprint(w, status.FormatHuman(report))
	return err
}
