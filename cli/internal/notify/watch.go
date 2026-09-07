package notify

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/blockadence/archimedes/cli/internal/contextmap"
	"github.com/blockadence/archimedes/cli/internal/prune"
	"github.com/blockadence/archimedes/cli/internal/stackref"
)

// DefaultStateFile is where a watch remembers what it has already reported,
// relative to the instance root. It sits beside repos.yaml because it
// describes one instance and is worth nothing anywhere else — delete it and
// the next pass simply reports the current backlog again.
const DefaultStateFile = ".archimedes-notify.json"

// Options is one watch pass.
type Options struct {
	// Root is the instance directory holding repos.yaml and work/.
	Root string
	// ContextFile overrides where each repo's context map lives, relative
	// to that repo's root. Empty means contextmap.DefaultContextFile.
	ContextFile string
	// StatePath overrides where this instance's state file lives. Empty
	// means <Root>/DefaultStateFile.
	StatePath string
	// Command is the operator's notification hook, run once per new
	// event. Empty delivers to out instead.
	Command string
	// Seed records what is true now without delivering any of it — how an
	// operator adopts a notifier on an instance with a backlog they
	// already know about.
	Seed bool
	// PRState looks up a unit of work's pull request state. Required:
	// nothing here can tell a merged branch from an open one on its own.
	PRState prune.PRStateFunc
	// Run executes the hook command. Empty means RunShell.
	Run Runner
}

// Watch reports every condition that has become true since the last pass:
// it collects what holds now, compares that against the state file, hands
// the difference to the operator's notifier, and records what it found for
// next time.
//
// It says nothing when there is no news. A watch is meant to be run by a
// scheduler every few minutes, and a scheduler mails whatever a job
// prints, so a pass that found nothing new prints nothing at all — the
// only mail an operator gets is mail worth reading.
//
// A hook that failed leaves its condition out of the recorded state and
// fails the pass. Both halves matter: the scheduler learns the notifier is
// broken, and the condition is still owed rather than filed away as news
// already broken to someone who never heard it.
func Watch(opts Options, out, progress io.Writer) error {
	if opts.PRState == nil {
		return errors.New("no pull request lookup configured")
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return fmt.Errorf("resolving instance root %s: %w", opts.Root, err)
	}
	statePath := opts.StatePath
	if statePath == "" {
		statePath = filepath.Join(root, DefaultStateFile)
	}

	current, err := Conditions(root, opts.ContextFile, opts.PRState, progress)
	if err != nil {
		return err
	}
	recorded := StateOf(current)

	if opts.Seed {
		if err := SaveState(statePath, recorded); err != nil {
			return err
		}
		fmt.Fprintf(out, "Recorded %d open condition(s) in %s without notifying about any of them.\n", len(current), statePath)
		return nil
	}

	previous, err := LoadState(statePath)
	if err != nil {
		return err
	}

	deliver := ToWriter(out)
	if opts.Command != "" {
		run := opts.Run
		if run == nil {
			run = RunShell
		}
		deliver = ToCommand(run, opts.Command)
	}

	fired := Since(previous, current)
	failures := 0
	for _, e := range fired {
		if err := deliver(e); err != nil {
			// Reported as it happens, and on progress rather than
			// carried in the returned error: one broken hook shouldn't
			// bury the others' detail in a single line, and everything
			// this pass prints reaches the scheduler's log anyway.
			fmt.Fprintf(progress, "warning: %v\n", err)
			recorded.Drop(e.Key())
			failures++
		}
	}

	// Written whatever happened: the conditions that were delivered are
	// news already broken, and holding the whole file back over one
	// failed hook would re-deliver all of them next pass.
	if err := SaveState(statePath, recorded); err != nil {
		return err
	}
	if len(fired) > failures {
		fmt.Fprintf(out, "\n%d new, %d open condition(s) in total.\n", len(fired)-failures, len(current))
	}
	if failures > 0 {
		return fmt.Errorf("%d of %d notifications could not be delivered; they stay unreported and will be retried next pass", failures, len(fired))
	}
	return nil
}

// Conditions collects everything currently worth notifying about in the
// instance at root: every tracked repo whose context map has gone stale,
// and every spawned worktree that is ready to be pruned.
//
// Both are read the way the subcommand that owns them reads it —
// contextmap.Survey and prune.Scan — so a watch can't come to a different
// conclusion than the `context-map` or `prune` run the operator makes in
// response to it.
//
// A repo that couldn't be assessed at all (an unfetchable remote, a
// checkout git doesn't recognize) is reported on progress and left out.
// Silence about one repo is the right failure for a pass a scheduler runs
// unattended: the alternative is either inventing a condition or dropping
// every other repo's news over one bad remote.
func Conditions(root, contextFile string, prState prune.PRStateFunc, progress io.Writer) ([]Event, error) {
	states, err := contextmap.Survey(root, contextFile, progress)
	if err != nil {
		return nil, err
	}

	var events []Event
	for _, s := range states {
		if s.Err != nil {
			fmt.Fprintf(progress, "note: could not assess %s: %v\n", s.Repo.Name, s.Err)
			continue
		}
		if !s.NeedsMapping() {
			continue
		}
		events = append(events, Event{
			Kind:    ContextStale,
			Subject: s.Repo.Name,
			Detail:  s.Reason,
			Remedy:  "archimedes context-map",
		})
	}

	items, err := prune.Scan(filepath.Join(root, "work"), "", prState)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		// A merged unit of work something else is still stacked on is not
		// prune-eligible — prune would refuse it — so it isn't news yet.
		// It becomes news once the dependent is rebased or pruned, which
		// is exactly when the operator can act on it.
		if !it.Prunable() {
			continue
		}
		events = append(events, Event{
			Kind:    PruneEligible,
			Subject: stackref.Ref{Repo: it.Repo, Slug: it.Slug}.String(),
			Detail:  it.PRState,
			Remedy:  fmt.Sprintf("archimedes prune %s --force", it.Slug),
		})
	}

	return events, nil
}
