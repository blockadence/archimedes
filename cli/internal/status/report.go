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
	// NeedsRebase marks a stacked branch whose base has merged out from
	// under it (see NeedsRebase in stack.go). RebaseOnto is the ref it
	// should be moved onto; it's empty on every unflagged row.
	NeedsRebase bool   `json:"needs_rebase"`
	RebaseOnto  string `json:"rebase_onto,omitempty"`
}

// Report is the full result of a status run: every row plus the guardrail
// verdict, everything the human table and the --json form both need.
type Report struct {
	Rows         []Row `json:"rows"`
	Count        int   `json:"count"`
	GuardrailMax int   `json:"guardrail_max"`
	GuardrailHit bool  `json:"guardrail_hit"`
}

// RepoRef is one repo as the instance manifest records it: where its
// checkout lives and which branch its work is based on.
type RepoRef struct {
	Path       string
	BaseBranch string
}

// Upstream is the remote-tracking ref this repo's branches are meant to
// sit on top of — the same origin/<base branch> spawn cuts them from.
func (r RepoRef) Upstream() string { return "origin/" + r.BaseBranch }

// RepoLookup resolves a repo name (as it appears in a status.md row) to
// its manifest entry, so its PR state can be looked up and its branches
// compared against the base branch they came from.
type RepoLookup func(repoName string) (RepoRef, error)

// Sources is everything a report reads the world through: the instance
// manifest, gh, and the repo checkouts on disk. Bundled into one value so
// a report can grow another source without every caller re-threading its
// arguments, and so tests substitute the parts they care about.
type Sources struct {
	Repos  RepoLookup
	PR     PRLookup
	Refs   GitRefs
	Merged MergedLookup
}

// RebaseNeeded returns the rows whose stacked base has merged, in report
// order.
func (r Report) RebaseNeeded() []Row {
	var flagged []Row
	for _, row := range r.Rows {
		if row.NeedsRebase {
			flagged = append(flagged, row)
		}
	}
	return flagged
}

// BuildReport looks up each entry's live PR state, flags any stacked row
// its base has merged out from under, and applies the guardrail
// threshold. A Repos or PR failure degrades that row to noPR rather than
// failing the whole report, matching status.sh (a gh_slug/gh failure for
// one row doesn't stop the others).
func BuildReport(entries []Entry, src Sources, guardrailMax int) Report {
	rows := make([]Row, 0, len(entries))
	for _, e := range entries {
		pr := noPR
		info, err := src.Repos(e.Repo)
		if err == nil {
			if looked, err := src.PR(info.Path, e.Slug); err == nil {
				pr = looked
			}
		}

		row := Row{
			Slug:     e.Slug,
			Repo:     e.Repo,
			PRNumber: pr.Number,
			PRState:  pr.State,
			Note:     e.Note,
		}
		if err == nil {
			row.NeedsRebase, row.RebaseOnto = checkStack(src, e, info)
		}
		rows = append(rows, row)
	}

	count := len(rows)
	return Report{
		Rows:         rows,
		Count:        count,
		GuardrailMax: guardrailMax,
		GuardrailHit: count > guardrailMax,
	}
}

// FormatHuman renders r as status.sh's human-readable table: the fixed-
// width column header and rows, a guardrail warning when it's hit, and a
// list of any stacked branches whose base has since merged.
func FormatHuman(r Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%-20s %-14s %-8s %-10s %-30s\n", "SLUG", "REPO", "PR#", "STATE", "NOTE")
	for _, row := range r.Rows {
		fmt.Fprintf(&b, "%-20s %-14s %-8s %-10s %-30s\n", row.Slug, row.Repo, row.PRNumber, row.PRState, row.Note)
	}

	b.WriteString("\n")
	if r.GuardrailHit {
		fmt.Fprintf(&b, "Warning: %d active worktree streams open, guardrail is %d. Consider closing some out.\n\n", r.Count, r.GuardrailMax)
	}

	if flagged := r.RebaseNeeded(); len(flagged) > 0 {
		b.WriteString("Rebase needed — these branches are stacked on a base that has since merged:\n")
		for _, row := range flagged {
			fmt.Fprintf(&b, "  %s / %s (%s) — rebase onto %s\n", row.Slug, row.Repo, row.Note, row.RebaseOnto)
		}
		b.WriteString("\n")
	}

	return b.String()
}
