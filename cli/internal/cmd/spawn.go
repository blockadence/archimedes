package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes/cli/internal/spawn"
)

// agentCmdEnvVar names the environment variable that overrides the agent
// CLI suggested by the next-step hint. Whatever agent the operator has
// configured, spawn suggests that one — never a hardcoded name.
const agentCmdEnvVar = "ARCHIMEDES_AGENT_CMD"

func newSpawnCmd() *cobra.Command {
	opts := spawn.Options{}
	var stackOn string

	cmd := &cobra.Command{
		Use:   "spawn <slug> <repo>",
		Short: "Create a branch+worktree for one unit of work in one target repo",
		Long: `Creates a branch+worktree for one unit of work in one target repo. Always
fetches first, so new branches start from current remote state, never from
a possibly-stale local checkout. The unit of work's reference material
(work/<slug>/) is automatically materialized into the new worktree.`,
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Slug, opts.Repo = args[0], args[1]
			opts.Stack = spawn.ParseStackRef(stackOn)
			opts.AgentCmd = os.Getenv(agentCmdEnvVar)
			return spawn.Run(opts, c.OutOrStdout(), c.ErrOrStderr())
		},
	}

	cmd.Flags().StringVar(&opts.Root, "root", ".", "instance root containing repos.yaml and work/")
	cmd.Flags().StringVar(&opts.Base, "base", "", "start the new branch from this ref instead of the repo's own base branch")
	cmd.Flags().StringVar(&stackOn, "stack-on", "", "start the new branch on top of another slug's branch, as <repo>:<slug>")

	return cmd
}
