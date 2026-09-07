package cmd

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes/cli/internal/notify"
	"github.com/blockadence/archimedes/cli/internal/prune"
)

// notifyCmdEnvVar names the operator's own notification hook, so an
// instance-wide choice lives in a shell profile rather than in every
// scheduler entry.
const notifyCmdEnvVar = "ARCHIMEDES_NOTIFY_CMD"

func newNotifyCmd() *cobra.Command {
	var root, statePath, command string
	var seed bool

	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Report context maps that have gone stale and worktrees ready to prune",
		Long: `Reports what has changed since the last run: a tracked repo whose context
map no longer matches its base branch's latest commit, and a spawned
worktree whose PR has merged or closed and that nothing else is stacked on.

Each condition is reported once, when it becomes true, and again only if it
clears and comes back — a map that goes staler while still unaddressed is
not news twice. Nothing stays resident to make that work: what has already
been reported lives in a state file beside repos.yaml (--state moves it),
which is the memory a background process would otherwise hold. Run it from
cron, launchd, or any other scheduler:

  */15 * * * * cd /path/to/instance && archimedes notify

A pass with nothing new prints nothing, so a scheduler that mails a job's
output mails you only when there is something to act on.

Set ARCHIMEDES_NOTIFY_CMD (or --command) to hand each notification to the
notifier you already run instead of printing it. It is run by sh once per
event, with the event in its environment — ARCHIMEDES_EVENT_TITLE,
ARCHIMEDES_EVENT_MESSAGE, ARCHIMEDES_EVENT_KIND, ARCHIMEDES_EVENT_SUBJECT,
ARCHIMEDES_EVENT_DETAIL, ARCHIMEDES_EVENT_REMEDY — and its title and
message on stdin:

  export ARCHIMEDES_NOTIFY_CMD='terminal-notifier -title "$ARCHIMEDES_EVENT_TITLE" -message "$ARCHIMEDES_EVENT_MESSAGE"'

Adopting this on an instance that already has a backlog you know about?
Run it once with --seed, which records what is true now and notifies about
none of it.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			opts := notifyOptions(root, statePath, command, seed, os.Getenv)
			return runNotify(c.OutOrStdout(), c.ErrOrStderr(), opts, prune.LookupPRState)
		},
	}

	cmd.Flags().StringVar(&root, "root", ".", "instance root containing repos.yaml and work/")
	cmd.Flags().StringVar(&statePath, "state", "", "where to remember what has already been reported (default <root>/"+notify.DefaultStateFile+")")
	cmd.Flags().StringVar(&command, "command", "", "shell command to deliver each notification with (default $"+notifyCmdEnvVar+", or print)")
	cmd.Flags().BoolVar(&seed, "seed", false, "record what is true now without notifying about any of it")

	return cmd
}

// notifyOptions assembles a watch from the flags plus the environment.
// The hook is the one setting an operator sets both ways — instance-wide
// in a profile, or for one run — so an explicit --command wins over the
// environment; everything else unset is left empty for internal/notify to
// apply its own default to.
func notifyOptions(root, statePath, command string, seed bool, env func(string) string) notify.Options {
	if command == "" {
		command = env(notifyCmdEnvVar)
	}
	return notify.Options{
		Root:        root,
		StatePath:   statePath,
		Command:     command,
		Seed:        seed,
		ContextFile: env(contextFileEnvVar),
	}
}

// runNotify fills in the one thing only this layer can — how a status.md
// row's repo name becomes a pull request lookup — and runs the pass.
// ghState is the underlying "owner/repo" lookup (production callers pass
// prune.LookupPRState; tests inject a fake so they need no gh session).
func runNotify(out, progress io.Writer, opts notify.Options, ghState prune.PRStateFunc) error {
	if err := requireBins("git", "gh"); err != nil {
		return err
	}

	m, err := loadManifest(opts.Root)
	if err != nil {
		return err
	}
	opts.PRState = repoPRState(opts.Root, m, ghState)

	return notify.Watch(opts, out, progress)
}
