package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/blockadence/gh-archimedes"
	"github.com/blockadence/gh-archimedes/internal/instance"
	"github.com/blockadence/gh-archimedes/internal/invocation"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <instance-name> <dest-parent-dir>",
		Short: "Scaffold a new instance from the built-in template",
		Long: `Creates <dest-parent-dir>/<instance-name> from the template carried in
this binary and gives it its own git history, so instance-specific
(possibly sensitive) content never shares a history with Archimedes.

Nothing else is needed on the machine: the template travels with the
binary, and what it creates is data — a manifest, dossier and work
directories, drivers and scaffolding the instance owns from here on.
Every command after this one acts on that data through this same install.

Refuses a destination that already exists rather than merging into it.

Where git has no identity configured — a fresh machine, a container, a CI
runner — the instance is still written, and the first commit is left for
you to make once you have set one. It is never made up: an instance is
your repository, and a fabricated author would stay in its history.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := instance.Create(archimedes.Template(), args[0], args[1])
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Instance ready at %s\n\n", res.Path)
			if !res.Committed {
				fmt.Fprintf(out, uncommittedNotice, res.Path, instance.CommitSubject(args[0]))
			}
			fmt.Fprintf(out, "Next: cd %s && %s bootstrap <github-org>\n", res.Path, invocation.Name())
			return nil
		},
	}
}

// uncommittedNotice is what init says when git had no identity to make the
// instance's first commit under. It is printed rather than returned as an
// error because nothing failed: the files — the valuable half of what init
// does — are all there, and the instance is usable as it stands. What is
// missing is a commit only its owner can author, so the notice hands over
// the four commands that finish the job, spelling out the same subject
// Create would have used rather than leaving the operator to invent one.
//
// Takes the instance path and the commit subject.
const uncommittedNotice = `Not committed: git has no identity configured, so the first commit was
skipped rather than made up. Set one, then make that commit yourself:

  git config --global user.name "Your Name"
  git config --global user.email "you@example.com"
  cd %s && git add -A && git commit -m "%s"

`
