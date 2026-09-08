// Command archimedes is the compiled entry point for cross-repo planning
// and git-worktree lifecycle orchestration. See internal/cmd for the
// subcommand tree.
package main

import (
	"context"
	"os"

	"github.com/blockadence/gh-archimedes/internal/cmd"
)

func main() {
	// fang has already rendered whatever came back, styled, on its way
	// out — so this only decides the exit status. Printing the error
	// again here would show every failure twice, and the second copy is
	// the one that mangles a multi-line message.
	//
	// Which status is cmd.ExitStatus's question: one for an ordinary
	// failure, and the driver's own for a run a signal stopped.
	if err := cmd.Execute(context.Background()); err != nil {
		os.Exit(cmd.ExitStatus(err))
	}
}
