package contextmap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blockadence/archimedes/cli/internal/gitutil"
	"github.com/blockadence/archimedes/cli/internal/manifest"
)

// RepoState is one repo's mapping state at a moment: where its checkout
// is, what its base branch currently points at, and whether the map on
// disk is still good for that commit.
//
// The three states a repo can be in are distinguished by which fields are
// set rather than by an enum, because each answers a different question:
// Err says the repo couldn't be assessed, Cloned says there's nothing to
// assess yet, and only past both of those does Assessment mean anything.
type RepoState struct {
	Repo manifest.Repo
	// Path is the repo's local checkout, empty until bootstrap clones it.
	Path string
	// CurrentSHA is the base branch's current commit on the remote.
	CurrentSHA string
	// OutputPath is where this repo's context map goes.
	OutputPath string
	// Err is why this repo couldn't be assessed — a fetch that failed, a
	// checkout git doesn't recognize — and is nil when it could.
	Err error
	Assessment
}

// Cloned reports whether the repo is on disk to be assessed at all.
func (s RepoState) Cloned() bool { return s.Path != "" }

// NeedsMapping reports whether this repo is one a pass would map: cloned,
// assessable, and not already current.
func (s RepoState) NeedsMapping() bool { return s.Cloned() && s.Err == nil && s.Stale }

// Inspect assesses whether repo's context map is still good for its base
// branch's current commit, without mapping anything.
//
// Staleness is measured against the remote's base branch, not the local
// checkout, so a stale local clone can't make a repo look current — which
// is why this fetches, and why it belongs to the pass rather than to
// Assess, which stays pure. git's own output goes to progress.
//
// A repo that isn't cloned yet, and a repo whose git commands fail, both
// come back described rather than as a returned error: a survey of an
// instance is worth more complete-with-gaps than abandoned at the first
// repo nobody has cloned. Callers that can't proceed past either — Run,
// which is about to hand the repo to a driver — check for themselves.
func Inspect(root, contextFile string, repo manifest.Repo, progress io.Writer) RepoState {
	state := RepoState{Repo: repo}

	path := filepath.Join(root, repo.Path)
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return state
	}
	state.Path = path
	state.OutputPath = filepath.Join(path, contextFile)

	if err := gitutil.RunOut(path, progress, "fetch", "origin", repo.BaseBranch, "-q"); err != nil {
		state.Err = err
		return state
	}
	currentSHA, err := gitutil.Run(path, "rev-parse", "origin/"+repo.BaseBranch)
	if err != nil {
		state.Err = err
		return state
	}

	state.CurrentSHA = currentSHA
	state.Assessment = Assess(repo.ContextModeledSHA, currentSHA, contextFile, isFile(state.OutputPath))
	return state
}

// Survey inspects every repo in the instance at root, in the dependency
// order a mapping pass would visit them, and reports what it found:
// which maps are stale, which are current, and which repos couldn't be
// assessed. It invokes no driver, launches no session, and writes nothing
// — it is what --dry-run reports on and what a watch (internal/notify)
// reads to notice a map going stale.
//
// contextFile is where each repo's map lives relative to its own root;
// empty means DefaultContextFile.
func Survey(root, contextFile string, progress io.Writer) ([]RepoState, error) {
	root, m, err := manifest.LoadInstance(root)
	if err != nil {
		return nil, err
	}
	if contextFile == "" {
		contextFile = DefaultContextFile
	}

	order, warning := Order(m.Repos)
	if warning != "" {
		fmt.Fprintln(progress, warning)
	}

	states := make([]RepoState, 0, len(order))
	for _, name := range order {
		repo, ok := m.Find(name)
		if !ok {
			continue
		}
		states = append(states, Inspect(root, contextFile, repo, progress))
	}
	return states, nil
}
