package bootstrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// listLimit caps how many repos one discovery pass considers, matching
// bootstrap.sh's `gh repo list --limit 300`.
const listLimit = 300

// defaultBaseBranch stands in for a repo gh reports no default branch for
// — an empty repo, most often — mirroring bootstrap.sh's
// `.defaultBranchRef.name // "main"`.
const defaultBaseBranch = "main"

// OrgRepo is one repo as the forge describes it: the fields
// `gh repo list --json name,sshUrl,defaultBranchRef,isArchived,isFork`
// returns, before Archimedes has decided anything about it. Distinct from
// manifest.Repo, which is what an instance goes on to record about it.
type OrgRepo struct {
	Name             string     `json:"name"`
	SSHURL           string     `json:"sshUrl"`
	DefaultBranchRef *branchRef `json:"defaultBranchRef"`
	IsArchived       bool       `json:"isArchived"`
	IsFork           bool       `json:"isFork"`
}

type branchRef struct {
	Name string `json:"name"`
}

// BaseBranch is the branch this repo's entry should record as its base.
func (r OrgRepo) BaseBranch() string {
	if r.DefaultBranchRef == nil || r.DefaultBranchRef.Name == "" {
		return defaultBaseBranch
	}
	return r.DefaultBranchRef.Name
}

// RepoLister reports the repos an org owns. Production callers pass
// ListOrgRepos; tests inject a fake so they need neither a gh session nor a
// network.
type RepoLister func(org string) ([]OrgRepo, error)

// filter narrows a discovery pass to the repos an instance should
// orchestrate. Forks are always dropped — work belongs in the upstream —
// and archived repos are dropped unless explicitly asked for.
func filter(repos []OrgRepo, includeArchived bool) []OrgRepo {
	var kept []OrgRepo
	for _, r := range repos {
		if r.IsFork || (r.IsArchived && !includeArchived) {
			continue
		}
		kept = append(kept, r)
	}
	return kept
}

// ListOrgRepos asks gh what org owns. Unlike a PR-state lookup, a failure
// here has nothing to fall back on — an empty list would read as "the org
// has no repos" and silently bootstrap nothing — so it's reported.
func ListOrgRepos(org string) ([]OrgRepo, error) {
	cmd := exec.Command("gh", "repo", "list", org,
		"--limit", fmt.Sprint(listLimit),
		"--json", "name,sshUrl,defaultBranchRef,isArchived,isFork")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// gh explains itself on stderr ("not logged in", "could not
		// resolve to an Organization"), which is the whole diagnosis
		// here — an exit status alone would leave the operator guessing.
		return nil, fmt.Errorf("listing %s's repos with gh: %w: %s", org, err, strings.TrimSpace(stderr.String()))
	}

	return parseRepoList(stdout.Bytes())
}

func parseRepoList(data []byte) ([]OrgRepo, error) {
	var repos []OrgRepo
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, fmt.Errorf("parsing gh repo list output: %w", err)
	}
	return repos, nil
}
