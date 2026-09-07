package spawn

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blockadence/archimedes/internal/dossier"
	"github.com/blockadence/archimedes/internal/gitutil"
)

// ContextDirName is the conventional, inside-the-worktree location for
// control-repo-owned reference material (a ticket, a spec, notes) that
// MaterializeContext copies in. Never committed to the target repo — see
// IgnoreWorktreeArtifacts.
const ContextDirName = ".archimedes"

// IgnoreWorktreeArtifacts makes ContextDirName invisible to `git
// status`/`git add -A` in every worktree of repoPath, without touching that
// repo's own tracked .gitignore. It writes to the repo's shared (commondir)
// info/exclude, which is local-only, untracked, and honored by every
// worktree sharing that repo — so it needs setting once per repo, and
// there's nothing to clean up when a worktree is later pruned.
// Check-then-append, so it assumes spawn isn't run concurrently for two
// slugs against the same repo (true of expected single-operator usage).
func IgnoreWorktreeArtifacts(repoPath string) error {
	commonDir, err := gitutil.CommonDir(repoPath)
	if err != nil {
		return err
	}

	excludeFile := filepath.Join(commonDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludeFile), 0o755); err != nil {
		return err
	}

	line := "/" + ContextDirName + "/"
	existing, err := os.ReadFile(excludeFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, l := range strings.Split(string(existing), "\n") {
		if l == line {
			return nil
		}
	}

	f, err := os.OpenFile(excludeFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(line + "\n")
	return err
}

// Context describes one worktree's materialization: which unit of work,
// into which repo's worktree, and which instance the reference material and
// dossiers come from.
type Context struct {
	// RepoPath is the target repo's checkout.
	RepoPath string
	// RepoName is the repo's name in repos.yaml, used to find its dossier.
	RepoName string
	// Root is the instance directory; work/ and repos/ live under it.
	Root string
	// Slug names the unit of work.
	Slug string
	// Worktree is the freshly created worktree to materialize into.
	Worktree string
}

// Materialize copies this unit of work's control-repo directory
// (work/<slug>/, whatever reference material it holds) into a freshly
// spawned worktree, at the conventional ContextDirName location, and
// guarantees it can never end up in a commit there. StatusFileName is
// Archimedes' own cross-repo bookkeeping (other worktrees' local paths for
// this slug), not reference material, so it's excluded from the copy.
//
// The target repo's house rules are delivered too, independently of whether
// this slug has any reference material of its own: house rules apply to
// every worktree of the repo, not just ones carrying a ticket. When there's
// neither, no ContextDirName directory is created at all.
func Materialize(c Context) error {
	if err := IgnoreWorktreeArtifacts(c.RepoPath); err != nil {
		return err
	}

	// A missing or unreadable work/<slug> is not an error — it just means
	// there's no reference material.
	src := filepath.Join(c.Root, "work", c.Slug)
	entries, err := os.ReadDir(src)
	hasWork := err == nil && len(entries) > 0

	rules, err := dossier.HouseRules(filepath.Join(c.Root, "repos"), c.RepoName)
	if err != nil {
		return err
	}

	if !hasWork && rules == "" {
		return nil
	}

	dest := filepath.Join(c.Worktree, ContextDirName)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	if hasWork {
		if err := copyTree(src, dest); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(dest, StatusFileName)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	if rules != "" {
		if err := os.WriteFile(filepath.Join(dest, dossier.HouseRulesFileName), []byte(rules+"\n"), 0o644); err != nil {
			return err
		}
	}

	return nil
}

// copyTree recursively copies src's contents into dest (both assumed to
// exist), preserving file contents, permissions, and symlinks, so the
// mechanism stays content-agnostic.
func copyTree(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		info, err := entry.Info()
		if err != nil {
			return err
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, destPath); err != nil {
				return err
			}
		case entry.IsDir():
			if err := os.MkdirAll(destPath, info.Mode().Perm()); err != nil {
				return err
			}
			if err := copyTree(srcPath, destPath); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
		// Anything else (device nodes, sockets) has no place in reference
		// material and is skipped rather than failing the spawn.
	}

	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
