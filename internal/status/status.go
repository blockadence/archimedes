// Package status reads the work/<slug>/status.md files spawn writes and
// reports each row's live PR state.
package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blockadence/gh-archimedes/internal/worktree"
)

// Entry is one repo row parsed out of a status.md table.
type Entry struct {
	Slug   string
	Repo   string
	Branch string
	// Worktree is the row's worktree column. ParseFile leaves it exactly
	// as the file records it — relative to the instance root, which is how
	// it is written (see internal/worktree) — and Discover resolves it
	// against the root it was given, since that is the layer that knows
	// which instance the file belongs to.
	Worktree string
	Note     string
}

// headerLines is the fixed title/blank/header/separator preamble every
// status.md starts with (see internal/spawn). Data rows start on the line
// after.
const headerLines = 4

// ParseFile parses one status.md's data rows into Entries, tagging each
// with slug (the work/<slug> directory name spawn keyed it under). It
// reports what the file says and nothing more: the worktree column comes
// back as recorded, since resolving it takes an instance root this layer
// is not given.
func ParseFile(path, slug string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) <= headerLines {
		return nil, nil
	}

	var entries []Entry
	for _, line := range lines[headerLines:] {
		fields := strings.Split(line, "|")
		if len(fields) < 5 {
			continue
		}

		repo := normalizeField(fields[1])
		if repo == "" {
			continue
		}

		entries = append(entries, Entry{
			Slug:     slug,
			Repo:     repo,
			Branch:   normalizeField(fields[2]),
			Worktree: normalizeField(fields[3]),
			Note:     normalizeField(fields[4]),
		})
	}

	return entries, nil
}

// Discover finds every work/<slug>/status.md in the instance at root and
// parses their rows, optionally restricted to a single slug. Each row's
// worktree comes back resolved against root, so a caller gets a path it
// can use rather than the relative one the file carries.
func Discover(root, slugFilter string) ([]Entry, error) {
	matches, err := filepath.Glob(filepath.Join(root, "work", "*", "status.md"))
	if err != nil {
		return nil, err
	}

	var all []Entry
	for _, m := range matches {
		slug := filepath.Base(filepath.Dir(m))
		if slugFilter != "" && slug != slugFilter {
			continue
		}

		entries, err := ParseFile(m, slug)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", m, err)
		}
		for i := range entries {
			entries[i].Worktree = worktree.Resolve(root, entries[i].Worktree)
		}
		all = append(all, entries...)
	}

	return all, nil
}

// normalizeField trims a table cell and collapses internal whitespace runs
// to a single space, so a hand-aligned table parses the same as a terse one.
func normalizeField(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// BranchName is the git branch a row refers to. spawn names the branch
// after the slug and records both, so the two normally agree; the branch
// column is what the row itself claims, so prefer it and fall back to the
// slug only for a row written without one.
func (e Entry) BranchName() string {
	if e.Branch != "" {
		return e.Branch
	}
	return e.Slug
}
