// Package status reads the work/<slug>/status.md files spawn writes and
// reports each row's live PR state.
//
// What a row says is internal/statusfile's answer, not this package's:
// prune acts on the same rows, and a report that read them its own way is
// how the two came to disagree (issue 58). What this package adds is
// everything the file does not know — the pull request behind a row, and
// whether a stacked branch has been left behind by its base merging.
package status

import "github.com/blockadence/gh-archimedes/internal/statusfile"

// Discover reads the rows of every work/<slug>/status.md in the instance
// at root, optionally narrowed to one slug.
//
// The narrowing is here rather than in the walk because prune's is not the
// same narrowing: it has to keep reading the files it is not reporting on.
// A report has no such tie between one unit of work and another.
//
// A row's worktree column comes back as the file records it — relative to
// the instance root (see internal/worktree) — and is not resolved on the
// way through. Nothing a report shows is a path: resolving belongs to the
// layer that hands one to git, which is prune.
func Discover(root, slugFilter string) ([]statusfile.Row, error) {
	files, err := statusfile.Discover(root)
	if err != nil {
		return nil, err
	}

	var rows []statusfile.Row
	for _, f := range files {
		if slugFilter != "" && f.Slug != slugFilter {
			continue
		}
		rows = append(rows, f.Rows...)
	}
	return rows, nil
}
