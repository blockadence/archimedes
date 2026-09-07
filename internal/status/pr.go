package status

import (
	"encoding/json"
	"os/exec"
	"strconv"

	"github.com/blockadence/gh-archimedes/internal/gitutil"
)

// PR is one row's live PR state, as reported by `gh pr list`.
type PR struct {
	Number string
	State  string
}

// noPR is what a row gets when it has no open or historical PR, or when
// the lookup itself couldn't be completed: a lookup failure is reported as
// "no PR" rather than raised, since gh/network flakiness shouldn't take
// down the whole report.
var noPR = PR{Number: "-", State: "no PR"}

// PRLookup resolves a repo's live PR state for a given head branch.
type PRLookup func(repoPath, headBranch string) (PR, error)

// GHLookup is the real PRLookup, backed by `git` and `gh`. Any failure
// (unresolvable remote, gh not installed, no network, no auth) degrades to
// noPR rather than an error.
func GHLookup(repoPath, headBranch string) (PR, error) {
	slug, err := gitutil.GHSlug(repoPath)
	if err != nil {
		return noPR, nil
	}

	out, err := exec.Command("gh", "pr", "list", "--repo", slug, "--head", headBranch, "--json", "number,state").Output()
	if err != nil {
		return noPR, nil
	}

	return parsePRList(out), nil
}

func parsePRList(data []byte) PR {
	var prs []struct {
		Number int    `json:"number"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(data, &prs); err != nil || len(prs) == 0 {
		return noPR
	}

	return PR{Number: strconv.Itoa(prs[0].Number), State: prs[0].State}
}

// GHMerged is the real MergedLookup: it asks gh directly whether
// headBranch has a merged pull request. `gh pr list` defaults to open pull
// requests only, so the state is asked for explicitly — a merged branch is
// exactly what wouldn't come back otherwise. Any failure (unresolvable
// remote, gh not installed, no network, no auth) reports "not merged", so
// an unanswerable lookup never turns into a rebase flag.
func GHMerged(repoPath, headBranch string) bool {
	slug, err := gitutil.GHSlug(repoPath)
	if err != nil {
		return false
	}

	out, err := exec.Command("gh", "pr", "list", "--repo", slug, "--head", headBranch, "--state", "merged", "--json", "number").Output()
	if err != nil {
		return false
	}
	return hasPR(out)
}

// hasPR reports whether a `gh pr list --json` payload holds at least one
// pull request.
func hasPR(data []byte) bool {
	var prs []struct{}
	if err := json.Unmarshal(data, &prs); err != nil {
		return false
	}
	return len(prs) > 0
}
