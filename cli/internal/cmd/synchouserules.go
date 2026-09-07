package cmd

import (
	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes/cli/internal/reposync"
)

func newSyncHouseRulesCmd() *cobra.Command {
	opts := reposync.HouseRulesOptions{}

	cmd := &cobra.Command{
		Use:   "sync-house-rules <repo>",
		Short: "Push one repo's house rules into it as a committed HOUSE_RULES.md",
		Long: `Pushes a repo's house rules — the "## House rules" section of its dossier,
repos/<repo>.md — into that repo as a durably committed HOUSE_RULES.md, via
a pull request. The dossier is the single source of truth: the same section
is what "archimedes spawn" injects into each worktree, so a house rule only
ever needs editing there.

Pass --dry-run to see the pending change without committing, pushing, or
opening a pull request.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireBins("git", "gh"); err != nil {
				return err
			}
			opts.Repo = args[0]
			return reposync.SyncHouseRules(opts, c.OutOrStdout(), c.ErrOrStderr(), reposync.RunCommand)
		},
	}

	cmd.Flags().StringVar(&opts.Root, "root", ".", "instance root containing repos.yaml and repos/")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show the pending change without committing, pushing, or opening a pull request")

	return cmd
}
