package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes/cli/internal/status"
)

// maxStreamsEnvVar names the environment variable holding the open-worktree
// guardrail threshold. It lives here with the CLI's other environment
// overrides rather than in internal/status, since which variable carries a
// setting is a CLI concern and the package that parses it shouldn't have to
// know.
const maxStreamsEnvVar = "ARCHIMEDES_MAX_STREAMS"

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
// ref, and merged-state sources; status.Collect fills in the rest.
func runStatus(w io.Writer, root, slugFilter string, jsonOutput bool, src status.Sources) error {
	report, err := status.Collect(root, slugFilter, src, status.ParseGuardrailMax(os.Getenv(maxStreamsEnvVar)))
	if err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	_, err = fmt.Fprint(w, status.FormatHuman(report))
	return err
}
