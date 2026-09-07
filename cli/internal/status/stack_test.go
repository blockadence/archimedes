package status

import "testing"

// stubRefs is a GitRefs backed by two lookup tables instead of a real
// checkout, so the decision logic can be exercised without building a repo
// per case.
type stubRefs struct {
	present   map[string]bool // "<repoPath>@<ref>"
	ancestors map[string]bool // "<repoPath>@<ancestor>..<descendant>"
}

func (s stubRefs) HasRef(repoPath, ref string) bool {
	return s.present[repoPath+"@"+ref]
}

func (s stubRefs) IsAncestor(repoPath, ancestor, descendant string) bool {
	if !s.HasRef(repoPath, ancestor) || !s.HasRef(repoPath, descendant) {
		return false
	}
	return s.ancestors[repoPath+"@"+ancestor+".."+descendant]
}

func merged(repoPath, headBranch string) bool    { return true }
func notMerged(repoPath, headBranch string) bool { return false }

const repo = "/repos/service-a"

// squashMerged is the state a squash merge leaves: "base"'s commits are
// still in "feature"'s history under their original SHAs, and origin/main
// has the same work under a new one — so base is nowhere on origin/main.
func squashMerged() stubRefs {
	return stubRefs{
		present: map[string]bool{
			repo + "@feature":     true,
			repo + "@base":        true,
			repo + "@origin/main": true,
		},
		ancestors: map[string]bool{repo + "@base..feature": true},
	}
}

func branches() (dep, base Branch) {
	dep = Branch{RepoPath: repo, Name: "feature", Upstream: "origin/main"}
	base = Branch{RepoPath: repo, Name: "base", Upstream: "origin/main"}
	return dep, base
}

func TestNeedsRebaseAfterTheBaseIsSquashMerged(t *testing.T) {
	dep, base := branches()

	if !NeedsRebase(squashMerged(), merged, dep, base) {
		t.Error("expected a rebase to be flagged: feature still carries base's commits, and origin/main has them under new SHAs")
	}
}

func TestNoRebaseOnceTheBranchHasBeenRebased(t *testing.T) {
	refs := squashMerged()
	// A rebase replays feature's own commits onto origin/main, so base's
	// commits leave its history.
	delete(refs.ancestors, repo+"@base..feature")
	refs.ancestors[repo+"@origin/main..feature"] = true
	dep, base := branches()

	if NeedsRebase(refs, merged, dep, base) {
		t.Error("expected no flag: base's commits are no longer in feature's history")
	}
}

func TestFlagStaysClearAsUpstreamMovesOn(t *testing.T) {
	refs := squashMerged()
	delete(refs.ancestors, repo+"@base..feature")
	// Someone else lands something, so origin/main is ahead of the rebased
	// branch again. Being behind main isn't what this flag reports.
	dep, base := branches()

	if NeedsRebase(refs, merged, dep, base) {
		t.Error("expected the flag to stay clear once rebased, even after origin/main moved on")
	}
}

func TestNoRebaseWhileTheBaseIsStillOpen(t *testing.T) {
	dep, base := branches()

	if NeedsRebase(squashMerged(), notMerged, dep, base) {
		t.Error("expected no flag: a healthy stack sits on its base's commits until the base merges")
	}
}

func TestNoRebaseWhenTheBaseLandedAsItself(t *testing.T) {
	refs := squashMerged()
	// A merge commit or fast-forward puts base's own commits on
	// origin/main, so feature and origin/main share them and feature's
	// pull request already shows only feature's work.
	refs.ancestors[repo+"@base..origin/main"] = true
	dep, base := branches()

	if NeedsRebase(refs, merged, dep, base) {
		t.Error("expected no flag: base landed under its own SHAs, so there's nothing to redo")
	}
}

func TestNoRebaseWhenTheUpstreamRefIsUnknown(t *testing.T) {
	refs := squashMerged()
	delete(refs.present, repo+"@origin/main")
	dep, base := branches()

	if NeedsRebase(refs, merged, dep, base) {
		t.Error("expected no flag: without origin/main there's no evidence either way")
	}
}

func TestNoRebaseWhenTheBaseBranchIsUnknownToTheDependentsRepo(t *testing.T) {
	refs := squashMerged()
	delete(refs.present, repo+"@base")
	dep, base := branches()

	if NeedsRebase(refs, merged, dep, base) {
		t.Error("expected no flag: nothing links this branch to the base any more")
	}
}

func TestNeedsRebaseAsksTheBasesOwnRepoWhetherItMerged(t *testing.T) {
	dep, base := branches()
	base.RepoPath = "/repos/service-b" // a cross-repo stack note

	var askedRepo, askedBranch string
	spy := func(repoPath, headBranch string) bool {
		askedRepo, askedBranch = repoPath, headBranch
		return true
	}

	if !NeedsRebase(squashMerged(), spy, dep, base) {
		t.Fatal("expected a rebase to be flagged")
	}
	if askedRepo != "/repos/service-b" || askedBranch != "base" {
		t.Errorf("merged state asked of %s@%s, want /repos/service-b@base", askedRepo, askedBranch)
	}
}
