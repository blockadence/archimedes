package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/blockadence/gh-archimedes/internal/gitutil"
	"github.com/blockadence/gh-archimedes/internal/prune"
)

func newPruneCmd() *cobra.Command {
	var root string
	var force bool

	cmd := &cobra.Command{
		Use:   "prune [slug]",
		Short: "Remove worktrees/branches whose PR merged or closed",
		Long: `Removes worktrees, branches, and status.md entries for units of work whose
PR has merged or closed. Refuses to remove a branch that's still acting as
another unit of work's stacked base — rebase that one onto the real base
branch first.

Dry run by default: lists candidates without touching anything. Pass
--force to actually remove them.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			slugFilter := ""
			if len(args) == 1 {
				slugFilter = args[0]
			}

			return runPrune(c.OutOrStdout(), root, slugFilter, force, prune.LookupPRState)
		},
	}

	cmd.Flags().StringVar(&root, "root", ".", "instance root containing repos.yaml and work/")
	cmd.Flags().BoolVar(&force, "force", false, "actually remove candidates instead of just listing them")

	return cmd
}

// runPrune scans root/work for merged/closed-PR candidates and, when
// force is set, removes them. ghState looks up a PR's state by "owner/repo"
// slug and head branch (production callers pass prune.LookupPRState; tests
// inject a fake so they don't need a real gh session).
func runPrune(out io.Writer, root, slugFilter string, force bool, ghState prune.PRStateFunc) error {
	if err := requireBins("git", "gh"); err != nil {
		return err
	}

	m, err := loadManifest(root)
	if err != nil {
		return err
	}

	items, err := prune.Scan(root, slugFilter, repoPRState(root, m, ghState))
	if err != nil {
		return err
	}

	for _, it := range items {
		if !it.Prunable() {
			fmt.Fprintf(out, "SKIP %s:%s (%s), still a base for: %s. Rebase that one first.\n",
				it.Repo, it.Slug, it.PRState, strings.Join(it.Blockers, ", "))
			continue
		}

		fmt.Fprintf(out, "PRUNE CANDIDATE: %s:%s (%s) at %s\n", it.Repo, it.Slug, it.PRState, it.Worktree)
		if !force {
			continue
		}

		repo, err := m.Resolve(root, it.Repo)
		if err != nil {
			return err
		}
		// gitutil's errors already name the git command and its
		// arguments, so re-wrapping here would just repeat the path.
		if err := gitutil.RemoveWorktree(repo.Path, it.Worktree); err != nil {
			return err
		}
		if err := gitutil.RemoveBranch(repo.Path, it.Slug); err != nil {
			return err
		}
		if err := prune.RemoveStatusRow(it.StatusPath, it.Repo); err != nil {
			return fmt.Errorf("updating %s: %w", it.StatusPath, err)
		}
		fmt.Fprintln(out, "  removed.")
	}

	if !force {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Dry run. Re-run with --force to actually remove the above.")
	}

	return nil
}
