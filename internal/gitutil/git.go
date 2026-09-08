// Package gitutil runs git as a subprocess on behalf of other packages that
// need repository state or need to mutate a working tree. It has no
// knowledge of Archimedes' own conventions (repos.yaml, work/, .archimedes/)
// — see internal/spawn for that. It does know the one thing every caller
// needs from a remote URL: the GitHub "owner/name" slug gh addresses repos
// by (GHSlug).
package gitutil

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Run executes git in dir and returns its trimmed stdout. On failure the
// returned error includes stderr.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RunOut executes git in dir with its output streamed to progress rather
// than captured, for commands whose progress is meant to reach the user
// directly (fetch, pull, worktree add). git writes progress to stderr, so
// both streams go to the one writer the caller designated for it — keeping
// them off the caller's own stdout.
func RunOut(dir string, progress io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = progress
	cmd.Stderr = progress
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// HasConfiguredIdentity reports whether git in dir has an author and a
// committer somebody set on purpose: in config, or in the
// GIT_AUTHOR_*/GIT_COMMITTER_* environment, which is configuration by
// another route and is where CI systems put it. It is for callers that
// commit into a repository belonging to an operator who did not type the
// commit, and so have to know not merely whether a commit would succeed but
// whose name it would carry.
//
// It is deliberately stricter than the commit it guards, which is the whole
// of what it is for. Where neither config nor environment names anybody, git
// derives an identity from the OS account and commits under it if what comes
// back is usable — a full name and login@hostname on a developer's macOS
// box, nothing at all on a fresh container or CI runner. For a commit the
// operator typed, that is the right answer and git's own: their machine,
// their repository, their guess to live with. For a commit they did not
// type, into a repository they will carry the history of, it is not: the
// author would be one nobody chose, and it stays.
//
// So the question is narrowed rather than answered ourselves.
// `user.useConfigOnly` is git's own switch for precisely this distinction —
// identity from configuration, no guessing — which keeps this git's answer
// to a stricter question rather than a rule of ours that would have to
// re-derive where git looks, and would sooner or later forget the
// environment and refuse on the CI systems that set one there deliberately.
//
// Both idents are checked because a commit needs both, and the environment
// can supply one without the other.
//
// So a false here is not the news that git cannot commit, and a caller that
// passes it on as that news will be wrong on every machine that guesses.
// What it has to say is that the commit was skipped and that git would have
// gone ahead — see internal/cmd's uncommittedNotice, which is that sentence
// written out.
func HasConfiguredIdentity(dir string) bool {
	for _, ident := range []string{"GIT_AUTHOR_IDENT", "GIT_COMMITTER_IDENT"} {
		if _, err := Run(dir, "-c", "user.useConfigOnly=true", "var", ident); err != nil {
			return false
		}
	}
	return true
}

// CommonDir returns the absolute path of repoPath's shared (commondir) git
// directory — the same directory across every worktree of that repo.
func CommonDir(repoPath string) (string, error) {
	out, err := Run(repoPath, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(out) {
		return out, nil
	}
	return filepath.Join(repoPath, out), nil
}

// Clone clones url into dest, with git's progress streamed to progress
// rather than captured — a clone is slow enough that the caller's user
// wants to see it happening.
func Clone(url, dest string, progress io.Writer) error {
	return RunOut("", progress, "clone", url, dest)
}

// RemoveWorktree force-removes the git worktree at wt from the repo
// checked out at repoPath, mirroring `git worktree remove --force`.
func RemoveWorktree(repoPath, wt string) error {
	_, err := Run(repoPath, "worktree", "remove", wt, "--force")
	return err
}

// RemoveBranch deletes branch from repoPath. An already-gone branch is not
// an error: the caller wanted it gone, and it is.
func RemoveBranch(repoPath, branch string) error {
	_, _ = Run(repoPath, "branch", "-D", branch)
	return nil
}

// githubRemoteRE extracts "owner/name" from a github.com origin remote URL,
// SSH or HTTPS — the form gh wants for --repo.
var githubRemoteRE = regexp.MustCompile(`github\.com[:/](.+)\.git$`)

// GHSlug derives the "owner/name" slug gh needs from repoPath's origin
// remote. If the remote URL doesn't match the expected github.com/...git
// shape, it's returned unchanged, same as the sed pass it replaces (which
// leaves non-matching input untouched rather than erroring).
func GHSlug(repoPath string) (string, error) {
	url, err := Run(repoPath, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("reading origin remote for %s: %w", repoPath, err)
	}

	if m := githubRemoteRE.FindStringSubmatch(url); m != nil {
		return m[1], nil
	}
	return url, nil
}

// HasRef reports whether ref names a commit in the repo at repoPath. Any
// failure to resolve it — a deleted branch, a remote-tracking ref that was
// never fetched, a broken repo — reads as absent, since every caller is
// asking "is this still here?" rather than "why not?".
func HasRef(repoPath, ref string) bool {
	_, err := Run(repoPath, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// IsAncestor reports whether ancestor is reachable from descendant, i.e.
// whether descendant's history already contains it (`git merge-base
// --is-ancestor`). A ref that doesn't resolve is neither an ancestor nor a
// descendant of anything, so an unknown ref reads as false rather than as
// an error — callers use this to decide whether a branch has been left
// behind, and "can't tell" must never masquerade as "yes".
func IsAncestor(repoPath, ancestor, descendant string) bool {
	if !HasRef(repoPath, ancestor) || !HasRef(repoPath, descendant) {
		return false
	}
	_, err := Run(repoPath, "merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}
