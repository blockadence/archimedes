package worktree_test

import (
	"path/filepath"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/worktree"
)

func TestPathIsASiblingOfTheCheckout(t *testing.T) {
	got := worktree.Path("/instance/target-repo", "widget-fix")
	want := "/instance/target-repo-worktrees/widget-fix"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The sibling layout bootstrap produces: repos beside the instance, their
// worktrees beside them again. What is written down is the same "../" shape
// repos.yaml carries for the checkout itself.
func TestRecordIsRelativeToTheInstance(t *testing.T) {
	root := "/Users/someone/Code/widgets"
	wt := worktree.Path("/Users/someone/Code/service-a", "widget-fix")

	got := worktree.Record(root, wt)
	want := "../service-a-worktrees/widget-fix"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if resolved := worktree.Resolve(root, got); resolved != wt {
		t.Errorf("round trip lost the path: got %q, want %q", resolved, wt)
	}
}

// A repo checked out below the instance root is recorded the same way, with
// no "../" to it — repos.yaml permits that layout, so this has to survive it.
func TestRecordHandlesACheckoutBelowTheRoot(t *testing.T) {
	root := "/Users/someone/Code/widgets"
	wt := worktree.Path(filepath.Join(root, "repos", "service-a"), "widget-fix")

	got := worktree.Record(root, wt)
	want := "repos/service-a-worktrees/widget-fix"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if resolved := worktree.Resolve(root, got); resolved != wt {
		t.Errorf("round trip lost the path: got %q, want %q", resolved, wt)
	}
}

// Rows written before the column was relative are still read, so an
// existing instance keeps working on the machine that spawned into it.
func TestResolveLeavesAnAbsoluteRowAlone(t *testing.T) {
	recorded := "/Users/someone/Code/service-a-worktrees/widget-fix"
	if got := worktree.Resolve("/Users/someone/Code/widgets", recorded); got != recorded {
		t.Errorf("got %q, want the row's own path %q", got, recorded)
	}
}

// Nothing is recorded that cannot be read back: where no relative path
// exists (a worktree on another Windows volume), the absolute one is
// written and Resolve understands it as the old shape.
func TestRecordFallsBackToWhatItWasGiven(t *testing.T) {
	wt := worktree.Path("/Users/someone/Code/service-a", "widget-fix")
	if got := worktree.Record("relative-root", wt); worktree.Resolve("relative-root", got) != wt {
		t.Errorf("recorded %q, which does not resolve back to %q", got, wt)
	}
}
