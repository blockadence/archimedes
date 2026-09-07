package spawn_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/dossier"
	"github.com/blockadence/gh-archimedes/internal/spawn"
	"github.com/blockadence/gh-archimedes/internal/testrepo"
)

// materializeFixture is a minimal instance root plus one target repo and a
// worktree of it — the state spawn's "git worktree add" leaves behind,
// without the surrounding orchestration.
type materializeFixture struct {
	root     string // instance root; work/ and repos/ live under it
	repoPath string
	worktree string
	slug     string
}

func newMaterializeFixture(t *testing.T, slug string) materializeFixture {
	t.Helper()
	tmp := t.TempDir()

	f := materializeFixture{
		root:     filepath.Join(tmp, "instance"),
		repoPath: testrepo.Init(t, filepath.Join(tmp, "repo")),
		slug:     slug,
	}
	mustMkdirAll(t, f.root)

	f.worktree = spawn.WorktreePath(f.repoPath, slug)
	testrepo.Git(t, f.repoPath, "worktree", "add", f.worktree, "-b", slug)

	return f
}

// workSlug creates work/<slug>/ under the instance root and returns it.
func (f materializeFixture) workSlug(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(f.root, "work", f.slug)
	mustMkdirAll(t, dir)
	return dir
}

// writeDossier records repo's dossier with the given House rules body.
func (f materializeFixture) writeDossier(t *testing.T, repo, houseRules string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(f.root, "repos"))
	mustWriteFile(t, filepath.Join(f.root, "repos", repo+".md"),
		"# "+repo+"\n\n## House rules\n\n"+houseRules+"\n\n## Known gotchas\nn/a\n")
}

func (f materializeFixture) materialize(t *testing.T, repoName string) {
	t.Helper()
	err := spawn.Materialize(spawn.Context{
		RepoPath: f.repoPath,
		RepoName: repoName,
		Root:     f.root,
		Slug:     f.slug,
		Worktree: f.worktree,
	})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
}

// contextPath is a path inside the worktree's materialized context dir.
func (f materializeFixture) contextPath(parts ...string) string {
	return filepath.Join(append([]string{f.worktree, spawn.ContextDirName}, parts...)...)
}

func TestMaterializeCopiesReferenceMaterial(t *testing.T) {
	f := newMaterializeFixture(t, "widget-fix")
	src := f.workSlug(t)

	mustWriteFile(t, filepath.Join(src, "ticket.md"), "# Ticket: widgets are broken\n")
	mustWriteFile(t, filepath.Join(src, "mockup.png"), "\x89PNG\r\n\x1a\nfakebinarydata")
	// A nested directory, to prove the copy recurses.
	mustMkdirAll(t, filepath.Join(src, "notes"))
	mustWriteFile(t, filepath.Join(src, "notes", "call.md"), "notes\n")
	// Bookkeeping, not reference material — must not be copied.
	mustWriteFile(t, filepath.Join(src, spawn.StatusFileName), "bookkeeping")

	f.materialize(t, "target")

	ticket, err := os.ReadFile(f.contextPath("ticket.md"))
	if err != nil {
		t.Fatalf("ticket.md was not materialized: %v", err)
	}
	if string(ticket) != "# Ticket: widgets are broken\n" {
		t.Errorf("ticket.md content diverged: %q", ticket)
	}
	if _, err := os.ReadFile(f.contextPath("mockup.png")); err != nil {
		t.Errorf("mockup.png was not materialized: %v", err)
	}
	if _, err := os.ReadFile(f.contextPath("notes", "call.md")); err != nil {
		t.Errorf("nested notes/call.md was not materialized: %v", err)
	}
	if _, err := os.Stat(f.contextPath(spawn.StatusFileName)); !os.IsNotExist(err) {
		t.Error("status.md (bookkeeping) leaked into the materialized context")
	}
}

// The mechanism is meant to work regardless of what kind of artifact is
// being copied, so a symlink is carried over as a symlink rather than
// silently dropped or flattened into its target's contents.
func TestMaterializePreservesSymlinks(t *testing.T) {
	f := newMaterializeFixture(t, "widget-fix")
	src := f.workSlug(t)

	mustWriteFile(t, filepath.Join(src, "ticket.md"), "# Ticket\n")
	if err := os.Symlink("ticket.md", filepath.Join(src, "latest.md")); err != nil {
		t.Fatal(err)
	}

	f.materialize(t, "target")

	link := f.contextPath("latest.md")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("symlink was not materialized: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was flattened into a regular file")
	}
	if target, err := os.Readlink(link); err != nil || target != "ticket.md" {
		t.Errorf("symlink target = %q (err %v), want %q", target, err, "ticket.md")
	}
}

func TestMaterializeNoopWhenNothingToDeliver(t *testing.T) {
	f := newMaterializeFixture(t, "widget-fix") // no work/<slug>/, no dossier

	f.materialize(t, "target")

	if _, err := os.Stat(filepath.Join(f.worktree, spawn.ContextDirName)); !os.IsNotExist(err) {
		t.Errorf("expected no %s directory to be created", spawn.ContextDirName)
	}
}

// House rules apply to every worktree of a repo, not just ones carrying
// their own reference material — so they're delivered even when the slug
// has no work/<slug> content at all.
func TestMaterializeDeliversHouseRulesWithoutReferenceMaterial(t *testing.T) {
	f := newMaterializeFixture(t, "quiet-fix")
	rules := "Never rebase a shared branch.\nAll schema changes go through the migration tool, no exceptions."
	f.writeDossier(t, "has-rules", rules)

	f.materialize(t, "has-rules")

	got, err := os.ReadFile(f.contextPath(dossier.HouseRulesFileName))
	if err != nil {
		t.Fatalf("%s was not materialized: %v", dossier.HouseRulesFileName, err)
	}
	if string(got) != rules+"\n" {
		t.Errorf("house rules content diverged\n got: %q\nwant: %q", got, rules+"\n")
	}
}

// An empty House rules section means "none recorded" and must not produce
// an empty .archimedes/ directory.
func TestMaterializeSkipsEmptyHouseRulesSection(t *testing.T) {
	f := newMaterializeFixture(t, "another-fix")
	f.writeDossier(t, "no-rules", "")

	f.materialize(t, "no-rules")

	if _, err := os.Stat(filepath.Join(f.worktree, spawn.ContextDirName)); !os.IsNotExist(err) {
		t.Errorf("expected no %s directory for a repo with no house rules", spawn.ContextDirName)
	}
}

func TestMaterializeDeliversHouseRulesAlongsideReferenceMaterial(t *testing.T) {
	f := newMaterializeFixture(t, "widget-fix")
	mustWriteFile(t, filepath.Join(f.workSlug(t), "ticket.md"), "# Ticket\n")
	f.writeDossier(t, "target", "Never rebase a shared branch.")

	f.materialize(t, "target")

	if _, err := os.Stat(f.contextPath("ticket.md")); err != nil {
		t.Errorf("ticket.md was not materialized: %v", err)
	}
	if _, err := os.Stat(f.contextPath(dossier.HouseRulesFileName)); err != nil {
		t.Errorf("%s was not materialized: %v", dossier.HouseRulesFileName, err)
	}
}

func TestMaterializeArtifactInvisibleToGitStatusAndAdd(t *testing.T) {
	f := newMaterializeFixture(t, "widget-fix")
	mustWriteFile(t, filepath.Join(f.workSlug(t), "ticket.md"), "ticket")
	f.writeDossier(t, "target", "Never rebase a shared branch.")

	f.materialize(t, "target")

	if got := testrepo.GitOut(t, f.worktree, "status", "--porcelain"); got != "" {
		t.Errorf("git status surfaced the materialized context: %q", got)
	}

	testrepo.Git(t, f.worktree, "add", "-A")
	if got := testrepo.GitOut(t, f.worktree, "status", "--porcelain"); got != "" {
		t.Errorf("git add -A staged the materialized context: %q", got)
	}
}

func TestIgnoreWorktreeArtifactsIsIdempotent(t *testing.T) {
	f := newMaterializeFixture(t, "widget-fix")

	if err := spawn.IgnoreWorktreeArtifacts(f.repoPath); err != nil {
		t.Fatalf("1st call: %v", err)
	}
	if err := spawn.IgnoreWorktreeArtifacts(f.repoPath); err != nil {
		t.Fatalf("2nd call: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(f.repoPath, ".git", "info", "exclude"))
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
