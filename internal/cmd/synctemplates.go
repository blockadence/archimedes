package cmd

import (
	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes/internal/reposync"
)

func newSyncTemplatesCmd() *cobra.Command {
	opts := reposync.TemplatesOptions{}

	cmd := &cobra.Command{
		Use:   "sync-templates [repo]",
		Short: "Push the canonical PR/issue templates into every tracked repo",
		Long: `Pushes this instance's canonical PR and issue templates (scaffolding/) into
every repo listed in repos.yaml as a pull request, or into just the one
named repo. The multi-repo fan-out is delegated to multi-gitter; the repo
list comes from the instance's manifest, so there's no separate config.

Pass --dry-run to see which repos would receive template changes without
pushing anything.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireBins("git", "gh", "multi-gitter"); err != nil {
				return err
			}
			if len(args) == 1 {
				opts.Repo = args[0]
			}
			return reposync.SyncTemplates(opts, c.OutOrStdout(), c.ErrOrStderr(), reposync.RunCommand)
		},
	}

	cmd.Flags().StringVar(&opts.Root, "root", ".", "instance root containing repos.yaml and scaffolding/")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "report what would change without pushing branches or opening pull requests")

	return cmd
}
