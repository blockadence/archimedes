// Package spawn creates the branch+worktree for one unit of work in one
// target repo: a port of template/scripts/spawn.sh, including the
// worktree-context materialization from template/scripts/lib.sh (see
// materialize.go). Kept independent of cobra/CLI concerns so it can be
// unit-tested directly.
package spawn

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blockadence/archimedes/cli/internal/gitutil"
	"github.com/blockadence/archimedes/cli/internal/manifest"
	"github.com/blockadence/archimedes/cli/internal/stackref"
)

// DefaultAgentCmd is the next-step hint's fallback when no agent CLI is
// configured.
const DefaultAgentCmd = "claude"

// Options is one spawn request: which unit of work, into which repo, and
// what to base it on.
type Options struct {
	// Root is the instance directory holding repos.yaml and work/.
	Root string
	// Slug names the unit of work; it becomes the branch name.
	Slug string
	// Repo is the target repo's name in repos.yaml.
	Repo string
	// Base, when set, overrides the repo's own base branch.
	Base string
	// Stack, when its Repo is set, stacks this branch on another slug's.
	Stack stackref.Ref
	// AgentCmd is the agent CLI the next-step hint should suggest. Empty
	// falls back to DefaultAgentCmd.
	AgentCmd string
}

// StartPoint is the resolved git ref a new branch is created from, plus a
// human-readable note recorded in the status file explaining the choice.
type StartPoint struct {
	Ref  string
	Note string
}

// ResolveStartPoint mirrors spawn.sh's precedence: a stack ref wins over a
// base override, which wins over the repo's own base branch (fetched fresh
// as origin/<baseBranch>). stack.Repo (not stack.Slug) is the presence
// check, matching the original script's `[ -n "$STACK_REPO" ]`.
func ResolveStartPoint(baseBranch, baseOverride string, stack stackref.Ref) StartPoint {
	switch {
	case stack.Repo != "":
		return StartPoint{Ref: stack.Slug, Note: stackref.Note(stack)}
	case baseOverride != "":
		return StartPoint{Ref: baseOverride, Note: fmt.Sprintf("based on %s", baseOverride)}
	default:
		return StartPoint{Ref: "origin/" + baseBranch, Note: fmt.Sprintf("based on %s", baseBranch)}
	}
}

// WorktreePath is where a repo's <slug> worktree lives: a sibling of the
// repo checkout itself, so it stays easy to find next to it without ever
// nesting inside it.
func WorktreePath(repoPath, slug string) string {
	return repoPath + "-worktrees/" + slug
}

// NextStepHint is the "what to do now" line printed after a successful
// spawn. agentCmd is whatever the operator configured, so no particular
// agent CLI is hardcoded.
func NextStepHint(worktreePath, agentCmd string) string {
	if agentCmd == "" {
		agentCmd = DefaultAgentCmd
	}
	return fmt.Sprintf("cd %s && %s", worktreePath, agentCmd)
}

// Run creates the branch and worktree described by opts. It always fetches
// first, so new branches start from current remote state rather than a
// possibly-stale local checkout, then materializes the unit of work's
// reference material into the new worktree and records it in the slug's
// status file.
//
// progress receives git's own output; out receives the result lines meant
// for the caller.
func Run(opts Options, out, progress io.Writer) error {
	// Absolutize up front, the way lib.sh's `ROOT="$(cd … && pwd)"` does.
	// Every path below derives from this, and git is run with its working
	// directory set to the target repo — so a relative root would resolve
	// worktree paths against the repo instead of the instance, nesting the
	// worktree inside the checkout it belongs beside.
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return fmt.Errorf("resolving instance root %s: %w", opts.Root, err)
	}

	manifestPath := filepath.Join(root, "repos.yaml")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	repo, ok := m.Find(opts.Repo)
	if !ok {
		return unknownRepoError(opts.Repo)
	}
	repoPath := filepath.Join(root, repo.Path)
	if info, err := os.Stat(repoPath); err != nil || !info.IsDir() {
		return unknownRepoError(opts.Repo)
	}

	if err := gitutil.RunOut(repoPath, progress, "fetch", "origin"); err != nil {
		return err
	}
	if err := gitutil.RunOut(repoPath, progress, "checkout", repo.BaseBranch); err != nil {
		return err
	}
	if err := gitutil.RunOut(repoPath, progress, "pull", "--ff-only", "origin", repo.BaseBranch); err != nil {
		return err
	}

	start := ResolveStartPoint(repo.BaseBranch, opts.Base, opts.Stack)

	wt := WorktreePath(repoPath, opts.Slug)
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		return err
	}
	if err := gitutil.RunOut(repoPath, progress, "worktree", "add", wt, "-b", opts.Slug, start.Ref); err != nil {
		return err
	}

	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(workDir, opts.Slug), 0o755); err != nil {
		return err
	}
	if err := Materialize(Context{
		RepoPath: repoPath,
		RepoName: opts.Repo,
		Root:     root,
		Slug:     opts.Slug,
		Worktree: wt,
	}); err != nil {
		return fmt.Errorf("materializing worktree context: %w", err)
	}

	if err := appendStatusRow(workDir, opts.Slug, opts.Repo, wt, start.Note); err != nil {
		return fmt.Errorf("recording status: %w", err)
	}

	fmt.Fprintf(out, "Worktree ready: %s (%s)\n", wt, start.Note)
	fmt.Fprintln(out, NextStepHint(wt, opts.AgentCmd))

	return nil
}

func unknownRepoError(name string) error {
	return fmt.Errorf("unknown repo: %s (run bootstrap first)", name)
}
