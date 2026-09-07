package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes/cli/internal/spawn"
	"github.com/blockadence/archimedes/cli/internal/stackref"
	"github.com/blockadence/archimedes/cli/internal/workspace"
)

// agentCmdEnvVar names the environment variable that overrides the agent
// CLI suggested by the next-step hint. Whatever agent the operator has
// configured, spawn suggests that one — never a hardcoded name.
const agentCmdEnvVar = "ARCHIMEDES_AGENT_CMD"

// workspaceEnvVar names the environment variable that opts spawn into a
// terminal workspace manager ("herdr"), so a new unit of work lands in a
// pane already rooted at its worktree. Unset, spawn creates the worktree
// and stops there, as it always has.
const workspaceEnvVar = "ARCHIMEDES_WORKSPACE"

// resolveWorkspace picks the terminal workspace manager for one spawn:
// the --workspace flag when given, otherwise whatever the environment
// configures instance-wide. The flag overrides in both directions, so
// `--workspace off` suppresses the pane for a single spawn (a scripted
// sweep across repos, say) without unsetting the environment.
func resolveWorkspace(flagValue string) (*workspace.Integration, error) {
	setting := flagValue
	if setting == "" {
		setting = os.Getenv(workspaceEnvVar)
	}
	return workspace.Select(setting)
}

func newSpawnCmd() *cobra.Command {
	opts := spawn.Options{}
	var stackOn, workspaceName string

	cmd := &cobra.Command{
		Use:   "spawn <slug> <repo>",
		Short: "Create a branch+worktree for one unit of work in one target repo",
		Long: `Creates a branch+worktree for one unit of work in one target repo. Always
fetches first, so new branches start from current remote state, never from
a possibly-stale local checkout. The unit of work's reference material
(work/<slug>/) is automatically materialized into the new worktree.

Opt in with --workspace (or ARCHIMEDES_WORKSPACE) to also open the new
worktree as a workspace in a terminal workspace manager, so the unit of
work arrives in a pane already rooted at its own checkout. Nothing is
opened by default, and a workspace manager that isn't installed or isn't
running degrades to a warning — the worktree is created either way.`,
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Slug, opts.Repo = args[0], args[1]
			opts.Stack = stackref.ParseFlag(stackOn)
			opts.AgentCmd = os.Getenv(agentCmdEnvVar)

			ws, err := resolveWorkspace(workspaceName)
			if err != nil {
				return err
			}
			opts.Workspace = ws

			return spawn.Run(opts, c.OutOrStdout(), c.ErrOrStderr())
		},
	}

	cmd.Flags().StringVar(&opts.Root, "root", ".", "instance root containing repos.yaml and work/")
	cmd.Flags().StringVar(&opts.Base, "base", "", "start the new branch from this ref instead of the repo's own base branch")
	cmd.Flags().StringVar(&stackOn, "stack-on", "", "start the new branch on top of another slug's branch, as <repo>:<slug>")
	cmd.Flags().StringVar(&workspaceName, "workspace", "",
		"open the new worktree in this terminal workspace manager (herdr, or off); defaults to $"+workspaceEnvVar)
	cmd.Flags().BoolVar(&opts.Focus, "focus", false,
		"switch to the new workspace instead of opening it in the background (with --workspace)")

	return cmd
}
