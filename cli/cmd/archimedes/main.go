// Command archimedes is the compiled entry point for cross-repo planning
// and git-worktree lifecycle orchestration. See internal/cmd for the
// subcommand tree.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/blockadence/archimedes/cli/internal/cmd"
)

func main() {
	if err := cmd.Execute(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
