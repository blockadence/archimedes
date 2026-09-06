package prune

import (
	"encoding/json"
	"os/exec"
)

type prListEntry struct {
	State string `json:"state"`
}

// parsePRListState extracts the lead PR's state from `gh pr list --json
// state` output, mirroring lib.sh's `--jq '.[0].state // "NONE"'`.
func parsePRListState(data []byte) string {
	var entries []prListEntry
	if err := json.Unmarshal(data, &entries); err != nil || len(entries) == 0 {
		return "NONE"
	}
	return entries[0].State
}

// LookupPRState asks gh for headBranch's PR state ("MERGED", "CLOSED",
// "OPEN", ...) against ghSlug ("owner/repo"). Any failure (no gh, no auth,
// no matching PR) comes back as "NONE" rather than an error, matching
// lib.sh's `gh pr list ... 2>/dev/null || echo NONE`.
func LookupPRState(ghSlug, headBranch string) (string, error) {
	out, err := exec.Command("gh", "pr", "list", "--repo", ghSlug, "--head", headBranch, "--json", "state").Output()
	if err != nil {
		return "NONE", nil
	}
	return parsePRListState(out), nil
}
