package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes/cli/internal/conventionpack"
	"github.com/blockadence/archimedes/cli/internal/manifest"
)

func newApplyConventionPackCmd() *cobra.Command {
	var root string

	cmd := &cobra.Command{
		Use:   "apply-convention-pack <repo>",
		Short: "Wire a repo up to the convention pack it declares",
		Long: `Adds whatever dependency or plugin reference a repo's build tool needs to
start pulling in the shared config artifact of the convention pack it
declares (repos.yaml's convention_pack field, defined in the instance's
convention-packs/).

One-time scaffolding, not ongoing sync: afterwards the repo owns that
reference like any other dependency, and nothing pushes updates back into
it later. Re-running against a repo already on the convention is a no-op,
and a build file already carrying a conflicting block of its own is
refused with instructions rather than rewritten.

The edit is left uncommitted in the target repo, for a human to review and
commit.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runApplyConventionPack(c.OutOrStdout(), c.ErrOrStderr(), root, args[0])
		},
	}

	cmd.Flags().StringVar(&root, "root", ".", "instance root containing repos.yaml and convention-packs/")

	return cmd
}

// runApplyConventionPack wires repoName up to the pack it declares. Both
// halves come from the instance itself — the pack name from its repos.yaml
// entry, the definition from the instance's convention-packs/ — so there's
// no separate config to keep in step with either.
//
// A build file that has to be edited by hand gets those instructions on
// errOut, where they keep their line breaks, and still fails the command.
func runApplyConventionPack(out, errOut io.Writer, root, repoName string) error {
	manifestPath := filepath.Join(root, "repos.yaml")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	// Both shortfalls name where the repo should have been: declaring a
	// convention pack means editing that same repos.yaml entry, and a repo
	// listed there but never cloned has nothing to scaffold onto yet.
	checkout := m.Checkout(root, repoName)
	if !checkout.Listed {
		return fmt.Errorf("unknown repo: %s (not in repos.yaml)", repoName)
	}
	if !checkout.Cloned {
		return fmt.Errorf("%s is in repos.yaml but not cloned yet (run archimedes bootstrap)", repoName)
	}
	repo := checkout.Repo
	if repo.ConventionPack == "" {
		return fmt.Errorf("%s has no convention_pack set in repos.yaml, nothing to do", repoName)
	}

	pack, err := conventionpack.Load(filepath.Join(root, conventionpack.DirName), repo.ConventionPack)
	if err != nil {
		return err
	}

	result, err := conventionpack.Apply(pack, conventionpack.Target{Name: repoName, Path: repo.Path})
	if err != nil {
		var manual *conventionpack.ManualEditError
		if errors.As(err, &manual) {
			fmt.Fprintln(errOut, manual.Instructions())
		}
		return err
	}

	fmt.Fprintln(out, result)
	return nil
}
