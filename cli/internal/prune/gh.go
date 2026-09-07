package prune

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

type prListEntry struct {
	State string `json:"state"`
}

// parsePRListState extracts the lead PR's state from `gh pr list --json
// state` output. No entries, or output that won't parse, is "NONE".
func parsePRListState(data []byte) string {
	var entries []prListEntry
	if err := json.Unmarshal(data, &entries); err != nil || len(entries) == 0 {
		return "NONE"
	}
	return entries[0].State
}

// LookupPRState asks gh for headBranch's PR state ("MERGED", "CLOSED",
// "OPEN", ...) against ghSlug ("owner/repo").
//
// A failure — no gh, no auth, a rate limit, no network — comes back as
// "NONE" and says why. "NONE" is what prune acts on, so a failure still
// can't be mistaken for permission to prune. The error is for the callers
// that need the other half of the answer: a branch with no pull request and
// a branch nobody could ask about look identical in the state alone, and a
// watch (internal/notify) that couldn't tell them apart would treat every
// gh outage as every merged unit of work being cleaned up.
//
// gh exits 0 with an empty list when a branch simply has no pull request,
// so a non-zero exit really does mean the question went unanswered.
func LookupPRState(ghSlug, headBranch string) (string, error) {
	out, err := exec.Command("gh", "pr", "list", "--repo", ghSlug, "--head", headBranch, "--json", "state").Output()
	if err != nil {
		return "NONE", fmt.Errorf("gh pr list --repo %s --head %s: %w", ghSlug, headBranch, err)
	}
	return parsePRListState(out), nil
}
