// Package contextmap sequences a context-mapping pass across every repo in
// an instance: dependency/base repos first, skipping any repo whose map is
// already current for its base branch's latest commit, so re-runs stay
// incremental as repos are added or merged into. It is the port of
// template/scripts/context-map-all.sh.
//
// It is orchestration only. Which repos need mapping and in what order
// lives here; how a map actually gets built does not — that's a driver's
// job (internal/driver), or, with no driver configured, an interactive
// session the operator runs themselves.
package contextmap

import (
	"fmt"
	"strings"

	"github.com/blockadence/archimedes/cli/internal/manifest"
)

// Order returns the names of repos in dependency order — every repo after
// the ones it depends on — via Kahn's algorithm: repeatedly take any
// not-yet-ordered repo whose dependencies are all already ordered.
//
// Repos that no dependency constrains keep their declared order, so the
// order stays predictable across runs. If a pass makes no progress, the
// remaining repos are in a cycle or depend on something undeclared; they're
// appended in declared order and described in the returned warning, so a
// bad depends_on degrades the ordering rather than stopping the pass.
func Order(repos []manifest.Repo) (order []string, warning string) {
	done := make(map[string]bool, len(repos))
	remaining := make([]manifest.Repo, len(repos))
	copy(remaining, repos)

	for len(remaining) > 0 {
		progressed := false
		var next []manifest.Repo

		for _, repo := range remaining {
			if !ready(repo, done) {
				next = append(next, repo)
				continue
			}
			// Marked done inside the pass, so a repo declared right
			// after its dependency is ready in the same pass.
			order = append(order, repo.Name)
			done[repo.Name] = true
			progressed = true
		}

		remaining = next
		if !progressed && len(remaining) > 0 {
			names := make([]string, len(remaining))
			for i, repo := range remaining {
				names[i] = repo.Name
				order = append(order, repo.Name)
			}
			return order, fmt.Sprintf("Cycle or unresolved dependency among: %s. Falling back to declared order.",
				strings.Join(names, " "))
		}
	}

	return order, ""
}

func ready(repo manifest.Repo, done map[string]bool) bool {
	for _, dep := range repo.DependsOn {
		if !done[dep] {
			return false
		}
	}
	return true
}

// Assessment is whether one repo needs a mapping pass, and why.
type Assessment struct {
	// Stale is true when the repo needs mapping.
	Stale bool
	// Reason explains why, for the operator: empty when not stale.
	Reason string
}

// Assess decides whether a repo's context map is still good for its base
// branch's current commit. A map counts as current only if the recorded
// commit matches and the file it produced is still on disk — a map that was
// recorded and then deleted needs rebuilding just as much as a stale one.
func Assess(storedSHA, currentSHA, contextFile string, contextFileExists bool) Assessment {
	switch {
	case storedSHA == currentSHA && contextFileExists:
		return Assessment{}
	case storedSHA == "":
		return Assessment{Stale: true, Reason: "never mapped"}
	case !contextFileExists:
		return Assessment{Stale: true, Reason: contextFile + " missing"}
	default:
		return Assessment{Stale: true, Reason: fmt.Sprintf("stale, %s -> %s", Short(storedSHA), Short(currentSHA))}
	}
}

// Short abbreviates a commit SHA for display, the way the shell's
// ${sha:0:8} did — including leaving anything shorter alone.
func Short(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

// SelectDriver resolves which driver builds one repo's map, most specific
// first: the repo's own driver field, then repos.yaml's instance-wide
// default, then the environment's (ARCHIMEDES_DRIVER). Empty means no
// driver is configured at any level, which falls back to an interactive
// session rather than to some default driver.
func SelectDriver(repoDriver, instanceDriver, environmentDriver string) string {
	if repoDriver != "" {
		return repoDriver
	}
	if instanceDriver != "" {
		return instanceDriver
	}
	return environmentDriver
}
