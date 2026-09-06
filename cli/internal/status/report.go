package status

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultGuardrailMax is status.sh's fallback threshold when
// ARCHIMEDES_MAX_STREAMS isn't set (or isn't a valid integer).
const DefaultGuardrailMax = 3

// ParseGuardrailMax parses raw (an ARCHIMEDES_MAX_STREAMS env value, or ""
// if unset) into a guardrail threshold, falling back to DefaultGuardrailMax
// when raw is empty or not a valid integer.
func ParseGuardrailMax(raw string) int {
	if raw == "" {
		return DefaultGuardrailMax
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return DefaultGuardrailMax
	}
	return n
}

// Row is one entry's rendered state, ready for either output form.
type Row struct {
	Slug     string `json:"slug"`
	Repo     string `json:"repo"`
	PRNumber string `json:"pr_number"`
	PRState  string `json:"pr_state"`
	Note     string `json:"note"`
}

// Report is the full result of a status run: every row plus the guardrail
// verdict, everything the human table and the --json form both need.
type Report struct {
	Rows         []Row `json:"rows"`
	Count        int   `json:"count"`
	GuardrailMax int   `json:"guardrail_max"`
	GuardrailHit bool  `json:"guardrail_hit"`
}

// RepoPath resolves a repo name (as it appears in a status.md row) to its
// local path, so its PR state can be looked up.
type RepoPath func(repoName string) (string, error)

// BuildReport looks up each entry's live PR state and applies the
// guardrail threshold. A RepoPath or PRLookup failure degrades that row to
// noPR rather than failing the whole report, matching status.sh (a
// gh_slug/gh failure for one row doesn't stop the others).
func BuildReport(entries []Entry, repoPath RepoPath, lookup PRLookup, guardrailMax int) Report {
	rows := make([]Row, 0, len(entries))
	for _, e := range entries {
		pr := noPR
		if path, err := repoPath(e.Repo); err == nil {
			if looked, err := lookup(path, e.Slug); err == nil {
				pr = looked
			}
		}

		rows = append(rows, Row{
			Slug:     e.Slug,
			Repo:     e.Repo,
			PRNumber: pr.Number,
			PRState:  pr.State,
			Note:     e.Note,
		})
	}

	count := len(rows)
	return Report{
		Rows:         rows,
		Count:        count,
		GuardrailMax: guardrailMax,
		GuardrailHit: count > guardrailMax,
	}
}

const todoTrailer = `TODO (not implemented yet): auto-detect stacked-rebase-needed. For any
'stacked on X:Y' note, check whether Y's worktree still exists; if
prune.sh already removed it (merged), this branch likely needs a
rebase onto the real base branch.`

// FormatHuman renders r as status.sh's human-readable table: the fixed-
// width column header and rows, a guardrail warning when it's hit, and the
// same "not implemented yet" trailer.
func FormatHuman(r Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%-20s %-14s %-8s %-10s %-30s\n", "SLUG", "REPO", "PR#", "STATE", "NOTE")
	for _, row := range r.Rows {
		fmt.Fprintf(&b, "%-20s %-14s %-8s %-10s %-30s\n", row.Slug, row.Repo, row.PRNumber, row.PRState, row.Note)
	}

	b.WriteString("\n")
	if r.GuardrailHit {
		fmt.Fprintf(&b, "Warning: %d active worktree streams open, guardrail is %d. Consider closing some out.\n", r.Count, r.GuardrailMax)
	}

	b.WriteString("\n")
	b.WriteString(todoTrailer)
	b.WriteString("\n")

	return b.String()
}
