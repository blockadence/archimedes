package spawn

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blockadence/archimedes/cli/internal/gitutil"
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

// MaterializeContext copies this unit of work's control-repo directory
// (workDir/slug/, whatever reference material it holds) into a freshly
// spawned worktree, at the conventional ContextDirName location, and
// guarantees it can never end up in a commit there. StatusFileName is
// Archimedes' own cross-repo bookkeeping (other worktrees' local paths for
// this slug), not reference material, so it's excluded from the copy.
func MaterializeContext(repoPath, workDir, slug, worktreePath string) error {
	if err := IgnoreWorktreeArtifacts(repoPath); err != nil {
		return err
	}

	// Nothing to copy is not an error, matching lib.sh's `[ ! -d "$src" ]`
	// guard — which treats a missing *or* non-directory source as a no-op.
	src := filepath.Join(workDir, slug)
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil
	}
	if len(entries) == 0 {
		return nil
	}

	dest := filepath.Join(worktreePath, ContextDirName)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if err := copyTree(src, dest); err != nil {
		return err
	}

	err = os.Remove(filepath.Join(dest, StatusFileName))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// copyTree recursively copies src's contents into dest (both assumed to
// exist), preserving file contents, permissions, and symlinks — matching
// what lib.sh's `cp -R "$src/."` carried over, so the mechanism stays
// content-agnostic.
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
