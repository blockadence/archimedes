package spawn

import (
	"fmt"
	"os"
	"path/filepath"
)

// StatusFileName is Archimedes' own cross-repo bookkeeping file inside
// work/<slug>/: which repos this unit of work spans and where each one's
// worktree lives, the latter recorded relative to the instance root like
// every other path an instance writes down (see internal/worktree). Not
// reference material, so materialize.go excludes it from what gets copied
// into a worktree.
const StatusFileName = "status.md"

func statusFilePath(workDir, slug string) string {
	return filepath.Join(workDir, slug, StatusFileName)
}

func statusHeader(slug string) string {
	return fmt.Sprintf("# %s\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n", slug)
}

func statusRow(repo, slug, worktree, note string) string {
	return fmt.Sprintf("| %s | %s | %s | %s | - |\n", repo, slug, worktree, note)
}

// appendStatusRow records one repo's worktree in work/<slug>/status.md,
// creating the file with its header first if this is the slug's first
// spawn.
func appendStatusRow(workDir, slug, repo, worktree, note string) error {
	path := statusFilePath(workDir, slug)

	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, []byte(statusHeader(slug)), 0o644); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(statusRow(repo, slug, worktree, note))
	return err
}
