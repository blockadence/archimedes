package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/blockadence/gh-archimedes/internal/contextmap"
	"github.com/blockadence/gh-archimedes/internal/dashboard"
	"github.com/blockadence/gh-archimedes/internal/invocation"
	"github.com/blockadence/gh-archimedes/internal/status"
)

func newDashboardCmd() *cobra.Command {
	var root string
	var refresh time.Duration

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Live, interactive view of worktree, PR and context-map state",
		Long: fmt.Sprintf(`Opens a live view of the whole instance: every spawned worktree with its
PR state and any rebase it's owed, alongside each repo's context-map
staleness. It refreshes on a timer, or on "r"; "q" quits.

Purely additive — a second way to look at what "%[1]s status" and
"%[1]s context-map --dry-run" already report, reading the same code
they do, so the two can never disagree. Every one of those commands
continues to work exactly as before, and nothing requires the dashboard.

Unlike a mapping pass, the dashboard never fetches: context-map staleness
is measured against origin/<base branch> as each checkout last saw it, so
looking costs no round trip and works offline. A repo nobody has fetched
lately can therefore under-report — it stays quiet about a pass that's due
rather than inventing one.

Needs a terminal to draw on; in a pipe or a log, use "%[1]s status".`, invocation.Name()),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runDashboard(c.OutOrStdout(), c.InOrStdin(), dashboardOptions(root, os.Getenv), refresh)
		},
	}

	cmd.Flags().StringVar(&root, "root", ".", "instance root containing repos.yaml and work/")
	cmd.Flags().DurationVar(&refresh, "refresh", dashboard.DefaultRefresh, "how often to retake the reading; 0 to refresh only on demand")

	return cmd
}

// dashboardOptions assembles a reading from the flags plus the same
// environment overrides the subcommands it mirrors honor, so the dashboard
// reports on the instance as those commands see it.
func dashboardOptions(root string, env func(string) string) dashboard.Options {
	return dashboard.Options{
		Root:         root,
		ContextFile:  env(contextFileEnvVar),
		GuardrailMax: status.ParseGuardrailMax(env(maxStreamsEnvVar)),
		Sources: status.Sources{
			PR:     status.GHLookup,
			Refs:   status.LocalRefs{},
			Merged: status.GHMerged,
		},
		SHA: contextmap.LocalSHA,
	}
}

// runDashboard starts the live view drawing on out and reading keys from
// in.
//
// A dashboard is the one subcommand that can't degrade into a pipe: it
// paints over itself and reads keystrokes. So rather than emit control
// codes into a log file, it points at the subcommand that answers the same
// question in a form a pipe can hold.
func runDashboard(out io.Writer, in io.Reader, opts dashboard.Options, refresh time.Duration) error {
	f, ok := out.(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) {
		return fmt.Errorf("dashboard needs an interactive terminal; use %q for the same data as a static table", invocation.Name()+" status")
	}

	_, err := tea.NewProgram(dashboard.New(opts, refresh), tea.WithOutput(out), tea.WithInput(in)).Run()
	return err
}
