// Package status reads the work/<slug>/status.md files spawn.sh writes and
// reports each row's live PR state, mirroring template/scripts/status.sh.
package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Entry is one repo row parsed out of a status.md table.
type Entry struct {
	Slug     string
	Repo     string
	Branch   string
	Worktree string
	Note     string
}

// headerLines is the fixed title/blank/header/separator preamble every
// status.md starts with (see spawn.sh). Data rows start on the line after.
const headerLines = 4

// ParseFile parses one status.md's data rows into Entries, tagging each
// with slug (the work/<slug> directory name spawn.sh keyed it under).
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

// Discover finds every work/<slug>/status.md under workDir and parses their
// rows, optionally restricted to a single slug.
func Discover(workDir, slugFilter string) ([]Entry, error) {
	matches, err := filepath.Glob(filepath.Join(workDir, "*", "status.md"))
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
		all = append(all, entries...)
	}

	return all, nil
}

// normalizeField trims a table cell and collapses internal whitespace runs
// to a single space, matching status.sh's `echo "$field" | xargs`.
func normalizeField(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
