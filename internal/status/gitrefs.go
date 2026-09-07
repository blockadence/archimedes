package status

import "github.com/blockadence/gh-archimedes/internal/gitutil"

// LocalRefs is the real GitRefs: it reads refs from the checkouts on disk
// via git, without fetching. A repo nobody has fetched recently reports on
// the origin/<base branch> it last saw, so the worst a stale checkout can
// do is stay quiet about a rebase that's due — never invent one.
type LocalRefs struct{}

func (LocalRefs) HasRef(repoPath, ref string) bool {
	return gitutil.HasRef(repoPath, ref)
}

func (LocalRefs) IsAncestor(repoPath, ancestor, descendant string) bool {
	return gitutil.IsAncestor(repoPath, ancestor, descendant)
}
