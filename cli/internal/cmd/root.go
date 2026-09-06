// Package cmd wires up the archimedes CLI's command tree. Add a new
// subcommand by adding a newXCmd() constructor here and registering it in
// newRootCmd.
package cmd

import (
	"context"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

// version is overridden at build time via:
//
//	go build -ldflags "-X github.com/blockadence/archimedes/cli/internal/cmd.version=$(git describe --tags)"
var version = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "archimedes",
		Short: "Cross-repo planning and worktree lifecycle orchestration",
		Long: `Archimedes orchestrates planning and git-worktree lifecycle across a
family of related repos: which repos a unit of work touches, and
tracking/spawning/pruning the worktrees used to execute it.`,
		SilenceUsage: true,
	}

	root.AddCommand(newRenderMapCmd())
	root.AddCommand(newStatusCmd())

	return root
}

// Execute runs the archimedes CLI, styled and augmented by fang (help,
// --version, shell completion, and man pages).
func Execute(ctx context.Context) error {
	return fang.Execute(ctx, newRootCmd(), fang.WithVersion(version))
}
