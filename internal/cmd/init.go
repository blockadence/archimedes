package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes"
	"github.com/blockadence/archimedes/internal/instance"
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

Refuses a destination that already exists rather than merging into it.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dest, err := instance.Create(archimedes.Template(), args[0], args[1])
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Instance ready at %s\n\n", dest)
			fmt.Fprintf(cmd.OutOrStdout(), "Next: cd %s && archimedes bootstrap <github-org>\n", dest)
			return nil
		},
	}
}
