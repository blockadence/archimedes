// Package prune finds and removes worktrees/branches whose PR has merged
// or closed — a port of template/scripts/prune.sh. Parsing and candidate
// selection are pure (Scan takes PR lookup as an injected function so they
// can be unit-tested without gh or a real git checkout); the git/gh-facing
// adapters that do the actual removal live in git.go and gh.go.
package prune

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Row is one data row of a work/<slug>/status.md table.
type Row struct {
	Repo     string
	Branch   string
	Worktree string
	Note     string
	PR       string
}

// parseRow parses one "| repo | branch | worktree | note | pr |" line.
// Splitting on "|" yields a leading and trailing empty field around the
// five columns, so a well-formed row has at least 6 fields.
func parseRow(line string) (Row, bool) {
	fields := strings.Split(line, "|")
	if len(fields) < 6 {
		return Row{}, false
	}
	repo := strings.TrimSpace(fields[1])
	if repo == "" {
		return Row{}, false
	}
	return Row{
		Repo:     repo,
		Branch:   strings.TrimSpace(fields[2]),
		Worktree: strings.TrimSpace(fields[3]),
		Note:     strings.TrimSpace(fields[4]),
		PR:       strings.TrimSpace(fields[5]),
	}, true
}

// ParseStatusFile parses a status.md's data rows, skipping the fixed
// 4-line header (title, blank, table header, separator) that spawn.sh
// always writes — the same convention prune.sh/status.sh's `tail -n +5`
// relies on.
func ParseStatusFile(data []byte) []Row {
	lines := strings.Split(string(data), "\n")
	if len(lines) <= 4 {
		return nil
	}

	var rows []Row
	for _, line := range lines[4:] {
		if row, ok := parseRow(line); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

// PRStateFunc looks up a head branch's PR state ("MERGED", "CLOSED",
// "OPEN", or "NONE") for repo, the way `gh pr list` does. A returned error
// is treated the same as "NONE" (no PR to act on) — a lookup failure
// (network, auth, no PR) must never be mistaken for permission to prune.
type PRStateFunc func(repo, headBranch string) (string, error)

// Item is one status.md row whose PR has merged or closed: a candidate
// for pruning, unless Blockers is non-empty.
type Item struct {
	Slug       string
	Repo       string
	Worktree   string
	Note       string
	PRState    string
	StatusPath string
	Blockers   []string // status.md paths that still name this repo:slug as a stacked base
}

// Prunable reports whether nothing still depends on this repo:slug as a
// stacked base.
func (it Item) Prunable() bool { return len(it.Blockers) == 0 }

// Scan walks workDir/*/status.md (optionally filtered to one slug) and
// returns every row whose PR has merged or closed. A row that's still
// named as another unit of work's stacked base ("stacked on repo:slug" in
// any status.md) comes back with Blockers set rather than being silently
// pruned out from under it.
func Scan(workDir, slugFilter string, prState PRStateFunc) ([]Item, error) {
	paths, err := filepath.Glob(filepath.Join(workDir, "*", "status.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	contents := make(map[string]string, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}
		contents[p] = string(data)
	}

	var items []Item
	for _, p := range paths {
		slug := filepath.Base(filepath.Dir(p))
		if slugFilter != "" && slug != slugFilter {
			continue
		}

		for _, row := range ParseStatusFile([]byte(contents[p])) {
			state, err := prState(row.Repo, slug)
			if err != nil {
				state = "NONE"
			}
			if state != "MERGED" && state != "CLOSED" {
				continue
			}

			target := fmt.Sprintf("stacked on %s:%s", row.Repo, slug)
			var blockers []string
			for other, content := range contents {
				if strings.Contains(content, target) {
					blockers = append(blockers, other)
				}
			}
			sort.Strings(blockers)

			items = append(items, Item{
				Slug:       slug,
				Repo:       row.Repo,
				Worktree:   row.Worktree,
				Note:       row.Note,
				PRState:    state,
				StatusPath: p,
				Blockers:   blockers,
			})
		}
	}

	return items, nil
}

// RemoveStatusRow deletes every row for repo from the status.md at path,
// mirroring prune.sh's `sed -i.bak "/| $repo |/d"`.
func RemoveStatusRow(path, repo string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if row, ok := parseRow(line); ok && row.Repo == repo {
			continue
		}
		out = append(out, line)
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}
