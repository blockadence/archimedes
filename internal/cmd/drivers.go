package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/blockadence/gh-archimedes"
	"github.com/blockadence/gh-archimedes/internal/driver"
	"github.com/blockadence/gh-archimedes/internal/invocation"
)

func newDriversCmd() *cobra.Command {
	var root string

	cmd := &cobra.Command{
		Use:   "drivers",
		Short: "List the context-mapping drivers this instance can run",
		Long: fmt.Sprintf(`Lists every driver a mapping pass or "run-driver" could resolve here, and
which of two places each one comes from.

Drivers under this instance's own drivers/ are the instance's: it is free
to edit them, and nothing ever overwrites or refreshes them. The rest ship
inside the archimedes binary, which is what lets a fix to one reach an
instance that already exists — upgrading the tool is the whole of the
update route. An instance driver sharing a name with a shipped one wins,
and is reported as shadowing it, since fixes to the shipped one stop
arriving there.

Use "%[1]s drivers adopt <name>" to take a shipped driver over.`, invocation.Name()),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return listDrivers(driverSet(root), c.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&root, "root", ".", "instance root whose drivers/ is searched before the ones archimedes ships")
	cmd.AddCommand(newDriversAdoptCmd())

	return cmd
}

func newDriversAdoptCmd() *cobra.Command {
	var root string

	cmd := &cobra.Command{
		Use:   "adopt <driver>",
		Short: "Copy a driver archimedes ships into this instance, to own and edit",
		Long: `Copies one of the drivers archimedes ships into this instance's drivers/,
where it takes precedence over the shipped one from then on.

This is how a shipped driver gets edited: adopt it, then change the copy.
It is a one-way, one-time act rather than a sync — the point of owning a
driver is that nothing refreshes it, which also means fixes made to the
shipped one no longer reach you. Adopting over a driver the instance
already has is refused rather than resolved, since that copy may be the
edit that was the reason for adopting.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			dest, err := driverSet(root).Adopt(args[0])
			if err != nil {
				// internal/driver knows which names it ships; only out
				// here is it known what the operator types to see them.
				if errors.Is(err, driver.ErrNotShipped) {
					return fmt.Errorf("%w (run `%s drivers` to see what does)", err, invocation.Name())
				}
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Adopted %s into %s\n", args[0], dest)
			fmt.Fprintf(c.OutOrStdout(), "It is this instance's now: edit it freely, and nothing will refresh it.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&root, "root", ".", "instance root whose drivers/ the driver is copied into")

	return cmd
}

// driverSet resolves drivers the one way every subcommand resolves them, so
// what `drivers` reports is exactly what a mapping pass would run.
func driverSet(root string) driver.Set {
	return driver.SetFor(root, os.Getenv(driversDirEnvVar), archimedes.Drivers())
}

func listDrivers(drivers driver.Set, out io.Writer) error {
	entries, err := drivers.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(out, "No drivers available.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tFROM\tDESCRIPTION")
	for _, e := range entries {
		from := string(e.Origin)
		if e.Shadows {
			from += " (shadows built-in)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", e.Name, from, describe(e))
	}
	if err := w.Flush(); err != nil {
		return err
	}

	if shadowed(entries) {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "A driver marked \"shadows built-in\" is this instance's own copy of one archimedes")
		fmt.Fprintln(out, "ships. It runs instead of the shipped one, and fixes to the shipped one will not")
		fmt.Fprintln(out, "reach it. Keep it if the copy is yours; delete drivers/<name>/ to go back to the")
		fmt.Fprintln(out, "one the tool maintains. An instance scaffolded before the drivers moved into the")
		fmt.Fprintln(out, "binary holds copies it never asked for, and its own drivers/README.md predates")
		fmt.Fprintf(out, "this -- `%s drivers --help` is the current answer.\n", invocation.Name())
	}
	return nil
}

func shadowed(entries []driver.Entry) bool {
	for _, e := range entries {
		if e.Shadows {
			return true
		}
	}
	return false
}

// describe is a driver's cell in the table: its description, or why it has
// none. A driver that won't load still gets a row, because this listing is
// what an operator runs when something is wrong.
func describe(e driver.Entry) string {
	if e.Err != nil {
		return "cannot be read: " + summarize(e.Err.Error())
	}
	return summarize(e.Description)
}

// summarize keeps the table a table. A manifest's description is meant to
// be one line, but nothing enforces it, and the folded YAML scalars the
// shipped drivers use arrive as one long line rather than a short one. The
// full text is in the manifest, and in drivers/README.md for the shipped
// ones; a listing is for picking a name out of a handful.
//
// Counted in runes, not bytes: this codebase writes em dashes everywhere
// and a driver's description is as likely to as anything else, and cutting
// one in half would print a replacement character rather than a word.
const summaryWidth = 64

func summarize(s string) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= summaryWidth {
		return string(r)
	}
	cut := summaryWidth
	for i := summaryWidth - 1; i > 0; i-- {
		if r[i] == ' ' {
			cut = i
			break
		}
	}
	return string(r[:cut]) + "..."
}
