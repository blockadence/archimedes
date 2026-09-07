// Package notify watches an instance for the two conditions an operator
// would otherwise only discover by remembering to run a status check: a
// repo's context map going stale, and a spawned worktree becoming
// prune-eligible.
//
// It is edge-triggered without staying resident. A watch pass compares
// what is true now against what the last pass recorded in a small state
// file, delivers only the difference, and exits — so the thing that has to
// keep running is an ordinary scheduler (cron, launchd, a CI cron job),
// not a daemon of ours. That file is the memory a long-lived process would
// otherwise hold in RAM, and the reason a machine that was asleep for a
// week reports each condition once rather than not at all or every time.
//
// Where a notification actually goes is the operator's business: with a
// hook command configured it is handed to whatever they already use
// (terminal-notifier, notify-send, ntfy, a Slack webhook), and with none
// it is printed, which is all a cron entry needs to turn it into mail.
package notify

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Kind is which of the watched conditions an event reports.
type Kind string

const (
	// ContextStale is a tracked repo whose context map no longer
	// describes its base branch's current commit.
	ContextStale Kind = "context-stale"
	// PruneEligible is a spawned worktree whose pull request has merged
	// or closed and which nothing else is still stacked on.
	PruneEligible Kind = "prune-eligible"
)

// Environment variables a hook command reads its event from. Names are
// exported because they are the hook contract: an operator writes a shell
// snippet against them, so they can't be quietly renamed.
const (
	EnvKind    = "ARCHIMEDES_EVENT_KIND"
	EnvSubject = "ARCHIMEDES_EVENT_SUBJECT"
	EnvDetail  = "ARCHIMEDES_EVENT_DETAIL"
	EnvRemedy  = "ARCHIMEDES_EVENT_REMEDY"
	EnvTitle   = "ARCHIMEDES_EVENT_TITLE"
	EnvMessage = "ARCHIMEDES_EVENT_MESSAGE"
)

// Event is one condition that is true about the instance right now: what
// kind of condition it is, what it is about, why it holds, and what to run
// about it.
type Event struct {
	Kind Kind `json:"kind"`
	// Subject is what the condition is about: a repo name, or the
	// "<repo>:<slug>" pair identifying one unit of work.
	Subject string `json:"subject"`
	// Detail is why it holds, in the words the subcommand that found it
	// would use — a staleness reason, a PR state.
	Detail string `json:"detail"`
	// Remedy is the archimedes command that acts on it, so a notification
	// arriving hours later doesn't need the operator to reconstruct what
	// to do about it.
	Remedy string `json:"remedy"`
}

// Key is the identity a condition keeps across runs, and so what decides
// whether an event is news.
//
// Detail is deliberately not part of it. A map that was stale for one
// commit and is now stale for a newer one is the same condition, still
// unaddressed, and notifying about it again on every base-branch push is
// how a notifier gets muted.
func (e Event) Key() string { return string(e.Kind) + ":" + e.Subject }

// Title is the one-line summary, for a desktop notification's title or
// the head of a line of output.
func (e Event) Title() string {
	switch e.Kind {
	case ContextStale:
		return "Context map stale: " + e.Subject
	case PruneEligible:
		return "Ready to prune: " + e.Subject
	default:
		return string(e.Kind) + ": " + e.Subject
	}
}

// Message is the body: why the condition holds and what to run about it.
func (e Event) Message() string {
	if e.Remedy == "" {
		return e.Detail
	}
	return fmt.Sprintf("%s. Run: %s", e.Detail, e.Remedy)
}

// Deliver hands one event to wherever notifications go on this machine.
type Deliver func(Event) error

// ToWriter delivers by printing one line per event. It is the fallback
// when no hook is configured, and it is what makes a cron entry with no
// configuration at all still worth having: cron mails whatever a job
// prints.
func ToWriter(w io.Writer) Deliver {
	return func(e Event) error {
		_, err := fmt.Fprintf(w, "%s — %s\n", e.Title(), e.Message())
		return err
	}
}

// Runner runs a hook command with env added to its environment and text on
// its stdin. It is the seam tests replace so they need no real notifier;
// production callers pass RunShell.
type Runner func(command string, env []string, stdin string) error

// ToCommand delivers by running the operator's own hook command once per
// event.
//
// The event reaches the hook through the environment and stdin, never
// interpolated into the command string. The command is the operator's own
// shell snippet, and a repo name, a slug, or a PR state has no business
// becoming part of it — that would make a branch name a way to run
// arbitrary shell on the machine watching it.
func ToCommand(run Runner, command string) Deliver {
	return func(e Event) error {
		env := []string{
			EnvKind + "=" + string(e.Kind),
			EnvSubject + "=" + e.Subject,
			EnvDetail + "=" + e.Detail,
			EnvRemedy + "=" + e.Remedy,
			EnvTitle + "=" + e.Title(),
			EnvMessage + "=" + e.Message(),
		}
		if err := run(command, env, e.Title()+"\n"+e.Message()+"\n"); err != nil {
			return fmt.Errorf("notification hook for %s: %w", e.Key(), err)
		}
		return nil
	}
}

// RunShell is the production Runner: the hook is run by sh, so an operator
// can configure a pipeline or a couple of commands rather than only a
// single executable. Its own output is left alone — a hook that has
// something to say (or a machine with no notifier where it fails) reaches
// the scheduler's log the same way anything else this run printed does.
func RunShell(command string, env []string, stdin string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sh -c %q: %w", command, err)
	}
	return nil
}
