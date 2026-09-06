package spawn_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/spawn"
)

// gitRepoWithWorktree sets up a git repo at <tmp>/repo with one commit on
// main, plus a worktree of it on a new branch — the shape spawn's "git
// worktree add" leaves behind, without the surrounding orchestration.
func gitRepoWithWorktree(t *testing.T, slug string) (repoPath, worktreePath string) {
	t.Helper()
	repoPath = filepath.Join(t.TempDir(), "repo")
	mustMkdirAll(t, repoPath)

	gitOK(t, repoPath, "init", "-q", "-b", "main")
	gitCommit(t, repoPath, "init", "--allow-empty")

	worktreePath = spawn.WorktreePath(repoPath, slug)
	gitOK(t, repoPath, "worktree", "add", worktreePath, "-b", slug)

	return repoPath, worktreePath
}

func TestMaterializeContextCopiesReferenceMaterial(t *testing.T) {
	slug := "widget-fix"
	repoPath, wt := gitRepoWithWorktree(t, slug)

	workDir := t.TempDir()
	src := filepath.Join(workDir, slug)
	mustMkdirAll(t, src)
	mustWriteFile(t, filepath.Join(src, "ticket.md"), "# Ticket: widgets are broken\n")
	mustWriteFile(t, filepath.Join(src, "mockup.png"), "\x89PNG\r\n\x1a\nfakebinarydata")
	// A nested directory, to prove the copy recurses.
	mustMkdirAll(t, filepath.Join(src, "notes"))
	mustWriteFile(t, filepath.Join(src, "notes", "call.md"), "notes\n")
	// Bookkeeping, not reference material — must not be copied.
	mustWriteFile(t, filepath.Join(src, spawn.StatusFileName), "bookkeeping")

	if err := spawn.MaterializeContext(repoPath, workDir, slug, wt); err != nil {
		t.Fatalf("MaterializeContext: %v", err)
	}

	context := filepath.Join(wt, spawn.ContextDirName)
	ticket, err := os.ReadFile(filepath.Join(context, "ticket.md"))
	if err != nil {
		t.Fatalf("ticket.md was not materialized: %v", err)
	}
	if string(ticket) != "# Ticket: widgets are broken\n" {
		t.Errorf("ticket.md content diverged: %q", ticket)
	}
	if _, err := os.ReadFile(filepath.Join(context, "mockup.png")); err != nil {
		t.Errorf("mockup.png was not materialized: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(context, "notes", "call.md")); err != nil {
		t.Errorf("nested notes/call.md was not materialized: %v", err)
	}
	if _, err := os.Stat(filepath.Join(context, spawn.StatusFileName)); !os.IsNotExist(err) {
		t.Error("status.md (bookkeeping) leaked into the materialized context")
	}
}

// The mechanism is meant to work regardless of what kind of artifact is
// being copied, so a symlink is carried over as a symlink rather than
// silently dropped or flattened into its target's contents.
func TestMaterializeContextPreservesSymlinks(t *testing.T) {
	slug := "widget-fix"
	repoPath, wt := gitRepoWithWorktree(t, slug)

	workDir := t.TempDir()
	src := filepath.Join(workDir, slug)
	mustMkdirAll(t, src)
	mustWriteFile(t, filepath.Join(src, "ticket.md"), "# Ticket\n")
	if err := os.Symlink("ticket.md", filepath.Join(src, "latest.md")); err != nil {
		t.Fatal(err)
	}

	if err := spawn.MaterializeContext(repoPath, workDir, slug, wt); err != nil {
		t.Fatalf("MaterializeContext: %v", err)
	}

	link := filepath.Join(wt, spawn.ContextDirName, "latest.md")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("symlink was not materialized: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink was flattened into a regular file")
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if target != "ticket.md" {
		t.Errorf("symlink target = %q, want %q", target, "ticket.md")
	}
}

func TestMaterializeContextNoopWhenNothingToCopy(t *testing.T) {
	slug := "widget-fix"
	repoPath, wt := gitRepoWithWorktree(t, slug)
	workDir := t.TempDir() // no work/<slug>/ at all

	if err := spawn.MaterializeContext(repoPath, workDir, slug, wt); err != nil {
		t.Fatalf("MaterializeContext: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, spawn.ContextDirName)); !os.IsNotExist(err) {
		t.Errorf("expected no %s directory to be created", spawn.ContextDirName)
	}
}

func TestMaterializeContextArtifactInvisibleToGitStatusAndAdd(t *testing.T) {
	slug := "widget-fix"
	repoPath, wt := gitRepoWithWorktree(t, slug)

	workDir := t.TempDir()
	src := filepath.Join(workDir, slug)
	mustMkdirAll(t, src)
	mustWriteFile(t, filepath.Join(src, "ticket.md"), "ticket")

	if err := spawn.MaterializeContext(repoPath, workDir, slug, wt); err != nil {
		t.Fatalf("MaterializeContext: %v", err)
	}

	if got := gitOut(t, wt, "status", "--porcelain"); got != "" {
		t.Errorf("git status surfaced the materialized context: %q", got)
	}

	gitOK(t, wt, "add", "-A")
	if got := gitOut(t, wt, "status", "--porcelain"); got != "" {
		t.Errorf("git add -A staged the materialized context: %q", got)
	}
}

func TestIgnoreWorktreeArtifactsIsIdempotent(t *testing.T) {
	repoPath, _ := gitRepoWithWorktree(t, "widget-fix")

	if err := spawn.IgnoreWorktreeArtifacts(repoPath); err != nil {
		t.Fatalf("1st call: %v", err)
	}
	if err := spawn.IgnoreWorktreeArtifacts(repoPath); err != nil {
		t.Fatalf("2nd call: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(repoPath, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if line == "/"+spawn.ContextDirName+"/" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one exclude line, found %d in %q", count, data)
	}
}
