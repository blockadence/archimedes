package contextmap

import (
	"io"
	"path/filepath"

	"github.com/blockadence/gh-archimedes/internal/gitutil"
	"github.com/blockadence/gh-archimedes/internal/manifest"
)

// SHALookup resolves the commit a repo's base branch is at, as seen from
// the checkout at repoPath.
//
// It exists so that "is this repo's map still current?" can be asked
// without also deciding how current the answer has to be. A mapping pass is
// about to spend a driver run on the answer, so it fetches first
// (FetchedSHA); a read-only reader wants an answer now and shouldn't drag
// the network into a screen refresh, so it reads what the checkout already
// has (LocalSHA).
type SHALookup func(repoPath, baseBranch string) (string, error)

// LocalSHA is the offline SHALookup: it reads origin/<baseBranch> exactly
// as the checkout last fetched it, without touching the network. A repo
// nobody has fetched in a while can therefore under-report staleness, which
// is the safe direction — it stays quiet about a mapping pass that's due
// rather than inventing one.
func LocalSHA(repoPath, baseBranch string) (string, error) {
	return gitutil.Run(repoPath, "rev-parse", "origin/"+baseBranch)
}

// FetchedSHA is the authoritative SHALookup: it fetches the base branch
// before reading it, so staleness is measured against the remote rather
// than against a local clone that may be behind — a stale checkout can't
// make a repo look current. git's own output goes to progress, since a
// fetch is slow enough that the operator wants to see it happening.
func FetchedSHA(progress io.Writer) SHALookup {
	return func(repoPath, baseBranch string) (string, error) {
		if err := gitutil.RunOut(repoPath, progress, "fetch", "origin", baseBranch, "-q"); err != nil {
			return "", err
		}
		return gitutil.Run(repoPath, "rev-parse", "origin/"+baseBranch)
	}
}

// RepoState is one repo's context-map state: where it lives, what it was
// last mapped at, where its base branch has got to, and the resulting
// verdict.
//
// Three of those are conditional on the ones before them, so read them in
// order: an entry that isn't Cloned has nothing to assess, and one whose
// Err is set couldn't be assessed. Assessment is only meaningful once both
// are clear — in particular a zero Assessment on such an entry means
// "unanswered", not "current".
type RepoState struct {
	Name string
	// Path is the repo's checkout, resolved against the instance root.
	Path string
	// ContextPath is where this repo's map lives (or would live).
	ContextPath string
	BaseBranch  string
	// Cloned reports whether Path is a directory yet: an entry can be in
	// repos.yaml before bootstrap has cloned it.
	Cloned bool
	// MappedSHA is the base-branch commit repos.yaml records the map as
	// built against; empty means never mapped.
	MappedSHA string
	// CurrentSHA is where the base branch actually is, per the SHALookup.
	// Empty when it couldn't be read.
	CurrentSHA string
	// Assessment is the verdict, from the same Assess every mapping pass
	// uses. Only meaningful when Cloned is true and Err is nil.
	Assessment
	// Err is why CurrentSHA couldn't be read — an unfetchable remote, a
	// base branch that doesn't exist there. Reported rather than returned,
	// so one unreadable repo doesn't take the whole survey down with it.
	Err error
}

// State resolves one repo against the instance at root and asks whether its
// context map is still current for its base branch's commit. It reads; it
// never maps anything or writes anything back.
func State(root string, repo manifest.Repo, contextFile string, sha SHALookup) RepoState {
	checkout := manifest.CheckoutOf(root, repo)
	state := RepoState{
		Name:        repo.Name,
		Path:        checkout.Path,
		ContextPath: filepath.Join(checkout.Path, contextFile),
		BaseBranch:  repo.BaseBranch,
		MappedSHA:   repo.ContextModeledSHA,
		Cloned:      checkout.Cloned,
	}
	if !state.Cloned {
		return state
	}

	current, err := sha(checkout.Path, repo.BaseBranch)
	if err != nil {
		state.Err = err
		return state
	}

	state.CurrentSHA = current
	state.Assessment = Assess(repo.ContextModeledSHA, current, contextFile, isFile(state.ContextPath))
	return state
}

// Survey assesses every repo in m, in the same dependency order a mapping
// pass would visit them in, and returns Order's warning alongside so a bad
// depends_on is as visible to a reader as it is to a pass.
//
// It is the read-only half of what Run does: same ordering, same
// per-repo verdict, no fetching or mapping unless sha does it.
func Survey(root string, m *manifest.Manifest, contextFile string, sha SHALookup) (states []RepoState, warning string) {
	order, warning := Order(m.Repos)

	states = make([]RepoState, 0, len(order))
	for _, name := range order {
		repo, ok := m.Find(name)
		if !ok {
			continue
		}
		states = append(states, State(root, repo, contextFile, sha))
	}

	return states, warning
}
