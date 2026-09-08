// Package dashboard is the optional live view of an instance: worktree and
// PR status alongside each repo's context-map staleness, refreshed in
// place instead of printed once.
//
// It is a presentation layer and nothing else. Every fact it shows is
// computed by the packages the plain subcommands already call —
// internal/status decides what a worktree row says and whether a stacked
// branch is owed a rebase, internal/contextmap decides whether a repo's map
// is still current — so the dashboard and the CLI can never disagree about
// the state of the instance, and turning the dashboard off costs nothing.
//
// The split inside the package is the same one: Collect gathers a Snapshot,
// Render turns one into a screen, and Model is the Bubble Tea loop that
// re-collects on a timer. Only Model needs a terminal; the other two are
// ordinary functions over data.
package dashboard

import (
	"fmt"
	"path/filepath"

	"github.com/blockadence/gh-archimedes/internal/contextmap"
	"github.com/blockadence/gh-archimedes/internal/manifest"
	"github.com/blockadence/gh-archimedes/internal/status"
)

// Options is where one reading of an instance reads the world through:
// which instance, and the same seams internal/status and
// internal/contextmap are driven by everywhere else, so tests can
// substitute gh and git without a live instance behind them.
type Options struct {
	// Root is the instance directory holding repos.yaml and work/.
	Root string
	// ContextFile is where each repo's context map lives, relative to that
	// repo's root. Empty means contextmap.DefaultContextFile.
	ContextFile string
	// GuardrailMax is the concurrent-stream threshold the worktree count
	// is judged against, the same one `status` applies.
	GuardrailMax int
	// Sources is how PR, ref and merged state are read. Its Repos is
	// filled in by Collect, which is the only layer that knows the root
	// the manifest resolves against.
	Sources status.Sources
	// SHA decides how current the base-branch commit staleness is measured
	// against has to be. Empty means contextmap.LocalSHA: a screen refresh
	// shouldn't wait on the network, and a repo nobody has fetched lately
	// under-reporting staleness is the safe direction.
	SHA contextmap.SHALookup
}

// Snapshot is one complete reading of an instance — everything a frame
// draws, with nothing left to compute.
type Snapshot struct {
	// Root is the instance the reading was taken from.
	Root string
	// Report is the worktree/PR table, identical to what `status` with no
	// slug filter would print.
	Report status.Report
	// Repos is every repo's context-map state, in the dependency order a
	// mapping pass would visit them in.
	Repos []contextmap.RepoState
	// OrderWarning is contextmap.Order's complaint about a dependency
	// cycle or an undeclared repo, empty when the order is sound.
	OrderWarning string
}

// Collect takes one reading of the instance at opts.Root.
//
// It fails only on what makes the whole reading meaningless — an
// unreadable repos.yaml, an unreadable work/ — because a dashboard that
// blanks itself over one unreachable repo is worse than one showing that
// repo as unknown. Everything narrower is carried in the snapshot: a PR
// lookup that couldn't reach gh degrades that row to "no PR", and a repo
// whose base branch couldn't be read carries its own error.
func Collect(opts Options) (Snapshot, error) {
	manifestPath := filepath.Join(opts.Root, "repos.yaml")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	src := opts.Sources
	src.Repos = status.ManifestRepos(m, opts.Root)

	rows, err := status.Discover(opts.Root, "")
	if err != nil {
		return Snapshot{}, fmt.Errorf("discovering status files: %w", err)
	}

	contextFile := opts.ContextFile
	if contextFile == "" {
		contextFile = contextmap.DefaultContextFile
	}
	sha := opts.SHA
	if sha == nil {
		sha = contextmap.LocalSHA
	}
	repos, warning := contextmap.Survey(opts.Root, m, contextFile, sha)

	return Snapshot{
		Root:         opts.Root,
		Report:       status.BuildReport(rows, src, opts.GuardrailMax),
		Repos:        repos,
		OrderWarning: warning,
	}, nil
}
