package status

import (
	"os"
	"path/filepath"
	"testing"
)

// writeStatus puts one unit of work's status.md where an instance keeps
// it, under root/work/<slug>/.
func writeStatus(t *testing.T, root, slug, worktree string) {
	t.Helper()
	slugDir := filepath.Join(root, "work", slug)
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# " + slug + "\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n" +
		"| service-a | " + slug + " | " + worktree + " | based on main | - |\n"
	if err := os.WriteFile(filepath.Join(slugDir, "status.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "alpha", "../service-a-worktrees/alpha")
	writeStatus(t, dir, "beta", "../service-a-worktrees/beta")

	t.Run("all slugs, in path order", func(t *testing.T) {
		got, err := Discover(dir, "")
		if err != nil {
			t.Fatalf("Discover returned error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 rows, got %d: %#v", len(got), got)
		}
		if got[0].Slug != "alpha" || got[1].Slug != "beta" {
			t.Errorf("got slugs %q, %q", got[0].Slug, got[1].Slug)
		}
		if got[0].Repo != "service-a" || got[0].Note != "based on main" {
			t.Errorf("row = %#v", got[0])
		}
	})

	t.Run("filtered to one slug", func(t *testing.T) {
		got, err := Discover(dir, "alpha")
		if err != nil {
			t.Fatalf("Discover returned error: %v", err)
		}
		if len(got) != 1 || got[0].Slug != "alpha" {
			t.Fatalf("expected 1 row for alpha, got %#v", got)
		}
	})

	t.Run("a slug nothing has been spawned into", func(t *testing.T) {
		got, err := Discover(dir, "never-spawned")
		if err != nil {
			t.Fatalf("Discover returned error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no rows, got %#v", got)
		}
	})

	t.Run("no status.md files yet", func(t *testing.T) {
		got, err := Discover(filepath.Join(dir, "nonexistent"), "")
		if err != nil {
			t.Fatalf("Discover returned error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no rows, got %#v", got)
		}
	})
}

// A report shows no paths, so the worktree column comes back as the file
// states it. Resolving it against an instance root is prune's, which is
// the layer that hands one to git.
func TestDiscoverReportsTheWorktreeColumnAsRecorded(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "alpha", "../service-a-worktrees/alpha")

	got, err := Discover(dir, "")
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got) != 1 || got[0].Worktree != "../service-a-worktrees/alpha" {
		t.Fatalf("got %#v, want the row's own recorded column", got)
	}
}

// A report on one unit of work reads that unit of work's file. Another
// slug's file being unreadable is not a reason to refuse the report that
// was asked for — and is a reason to refuse the whole-instance one, which
// would otherwise under-report an instance it could not fully read.
func TestDiscoverNarrowedToOneSlugDoesNotReadTheOthers(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "alpha", "../service-a-worktrees/alpha")
	// A status.md that cannot be read at all: a directory where the file
	// belongs.
	if err := os.MkdirAll(filepath.Join(dir, "work", "beta", "status.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(dir, "alpha")
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "alpha" {
		t.Fatalf("got %#v, want alpha's own row", got)
	}

	if _, err := Discover(dir, ""); err == nil {
		t.Error("a whole-instance report must fail on a status.md it cannot read, got nil")
	}
}
