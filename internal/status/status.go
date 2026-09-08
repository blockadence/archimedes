// Package status reads the work/<slug>/status.md files spawn writes and
// reports each row's live PR state.
//
// What a row says is internal/statusfile's answer, not this package's:
// prune acts on the same rows, and a report that read them its own way is
// how the two came to disagree (issue 58). What this package adds is
// everything the file does not know — the pull request behind a row, and
// whether a stacked branch has been left behind by its base merging.
package status

import (
	"os"

	"github.com/blockadence/gh-archimedes/internal/statusfile"
)

// Discover reads the rows of every work/<slug>/status.md in the instance
// at root, or of one slug's file when slugFilter names one.
//
// Narrowed, it reads that unit of work's file and no other. A report has
// no tie between one unit of work and another — unlike prune, which has to
// keep reading the files it is not acting on, since that is where a
// stacked base is named — so one slug's unreadable file must not be able
// to cost an operator the report on the slug they asked about.
//
// A row's worktree column comes back as the file records it — relative to
// the instance root (see internal/worktree) — and is not resolved on the
// way through. Nothing a report shows is a path: resolving belongs to the
// layer that hands one to git, which is prune.
func Discover(root, slugFilter string) ([]statusfile.Row, error) {
	if slugFilter != "" {
		file, err := statusfile.ReadFile(statusfile.Path(root, slugFilter), slugFilter)
		if err != nil {
			// A slug with no status.md is a slug nothing has been spawned
			// into: an empty report, not a failed one, the same answer the
			// walk gives for an instance with no work/ at all.
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		return file.Rows, nil
	}

	files, err := statusfile.Discover(root)
	if err != nil {
		return nil, err
	}

	var rows []statusfile.Row
	for _, f := range files {
		rows = append(rows, f.Rows...)
	}
	return rows, nil
}
