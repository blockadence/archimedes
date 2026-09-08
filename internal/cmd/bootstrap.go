package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/blockadence/gh-archimedes/internal/bootstrap"
)

func newBootstrapCmd() *cobra.Command {
	opts := bootstrap.Options{List: bootstrap.ListOrgRepos}

	cmd := &cobra.Command{
		Use:   "bootstrap <github-org>",
		Short: "Discover an org's repos, clone them, and scaffold their entries",
		Long: `Discovers a GitHub org's repos, clones the ones not already checked out
beside the instance, and scaffolds a repos.yaml entry plus a dossier stub
for each, then regenerates WORKSPACE-MAP.md.

Forks are skipped, and archived repos are skipped unless --include-archived
is passed. Safe to re-run: an already-bootstrapped instance only gains
what's missing, and nothing already written — a dossier, a declared
convention pack, a local checkout — is touched.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Org = args[0]
			return runBootstrap(opts, c.OutOrStdout(), c.ErrOrStderr())
		},
	}

	cmd.Flags().StringVar(&opts.Root, "root", ".", "instance root containing repos.yaml and repos/")
	cmd.Flags().BoolVar(&opts.IncludeArchived, "include-archived", false, "also track the org's archived repos")

	return cmd
}

// runBootstrap checks for what a bootstrap pass needs on the machine and
// then runs it. opts.List is what discovers the org's repos: production
// callers arrive here with bootstrap.ListOrgRepos, tests with a fake, so a
// test can scaffold a real instance without a gh session, a network, or an
// org that exists.
//
// out receives the result lines meant for the operator; progress receives
// git's own clone output.
func runBootstrap(opts bootstrap.Options, out, progress io.Writer) error {
	if err := requireBins("git", "gh"); err != nil {
		return err
	}
	return bootstrap.Run(opts, out, progress)
}
