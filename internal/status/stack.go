package status

import "github.com/blockadence/gh-archimedes/internal/stackref"

// GitRefs answers the questions stacked-rebase detection asks of a
// checkout. Both are reads of refs already on disk — no fetching — so a
// status run stays local and offline-safe; the cost is that a repo nobody
// has fetched in a while can under-report, which is the safe direction
// (see NeedsRebase).
type GitRefs interface {
	// HasRef reports whether ref names a commit in the repo at repoPath.
	HasRef(repoPath, ref string) bool
	// IsAncestor reports whether ancestor is reachable from descendant.
	IsAncestor(repoPath, ancestor, descendant string) bool
}

// MergedLookup reports whether headBranch's pull request in the repo
// checked out at repoPath has been merged. It exists because git alone
// can't answer that: a squash or rebase merge rewrites the branch's
// commits, so nothing on the base branch is recognizably the same objects.
// Anything short of a definite yes — no PR, no gh, no network — is a no,
// so an unanswerable lookup stays silent rather than inventing a flag.
type MergedLookup func(repoPath, headBranch string) bool

// Branch is one branch, the checkout it lives in, and the remote-tracking
// ref that checkout's work is meant to sit on top of (origin/<base
// branch>).
type Branch struct {
	RepoPath string
	Name     string
	Upstream string
}

// NeedsRebase reports whether dep — a branch stacked on base — is still
// carrying commits that upstream has already absorbed under different
// SHAs, and so would open a pull request re-proposing work that has
// already landed.
//
// Three things have to hold, cheapest first:
//
// Base's commits are still in dep's history. That's what a rebase strips
// out, so it's the condition that clears — permanently — the moment dep is
// rebased. Testing it against base's own tip rather than against
// dep's upstream is what makes it stable: upstream moves every time
// anything else lands, base's tip doesn't.
//
// Those commits are not on dep's upstream. If base landed as a merge
// commit or a fast-forward, its commits are on upstream as themselves, dep
// and upstream share them, and dep's pull request already shows only dep's
// own work — nothing to redo. Only a rewriting merge leaves dep holding
// duplicates.
//
// Base actually merged. Without this, every healthy stack — a base still
// open, its dependent sitting on top of it — matches the two conditions
// above. Only a merged base turns "sitting on top of it" into a problem,
// and only merged tells a landed branch apart from an abandoned one.
//
// Both git questions are asked of dep's own checkout, because that's where
// spawn resolved the start point when it created dep: --stack-on names the
// base's repo for bookkeeping, but the ref it branches from is the base's
// slug in the repo being spawned into.
func NeedsRebase(refs GitRefs, merged MergedLookup, dep, base Branch) bool {
	if !refs.HasRef(dep.RepoPath, dep.Upstream) {
		return false // never fetched: no evidence either way
	}
	if !refs.IsAncestor(dep.RepoPath, base.Name, dep.Name) {
		return false // already rebased off it, or never really on it
	}
	if refs.IsAncestor(dep.RepoPath, base.Name, dep.Upstream) {
		return false // landed as itself; dep isn't duplicating anything
	}
	return merged(base.RepoPath, base.Name)
}

// stackedBase resolves the base named by a "stacked on <repo>:<slug>" note
// into the two Branches NeedsRebase compares: the row's own branch, and
// the base it was cut from. Reports false for a note that isn't a stack
// note at all, or one naming a repo this instance doesn't track.
func stackedBase(e Entry, info RepoRef, repos RepoLookup) (dep, base Branch, ok bool) {
	ref, ok := stackref.ParseNote(e.Note)
	if !ok {
		return Branch{}, Branch{}, false
	}

	// The base's own checkout, which is where its pull request lives and
	// so where its merged state is decided.
	baseInfo, err := repos(ref.Repo)
	if err != nil {
		return Branch{}, Branch{}, false
	}

	dep = Branch{RepoPath: info.Path, Name: e.BranchName(), Upstream: info.Upstream()}
	base = Branch{RepoPath: baseInfo.Path, Name: ref.Slug, Upstream: baseInfo.Upstream()}
	return dep, base, true
}

// checkStack decides whether one status.md row is a stacked branch left
// behind by its base merging, returning the flag and the ref to rebase
// onto.
func checkStack(src Sources, e Entry, info RepoRef) (bool, string) {
	dep, base, ok := stackedBase(e, info, src.Repos)
	if !ok {
		return false, ""
	}
	if !NeedsRebase(src.Refs, src.Merged, dep, base) {
		return false, ""
	}
	return true, dep.Upstream
}
