package workdir_test

import (
	"testing"

	"github.com/blockadence/gh-archimedes/internal/workdir"
)

// The layout spelled out rather than built from the code under test: this
// is the one place that says where a unit of work's directory is, so the
// test that holds it has to state the answer itself.
func TestPathIsTheSlugDirectoryUnderTheInstancesWork(t *testing.T) {
	got := workdir.Path("/Users/someone/Code/widgets", "widget-fix")
	want := "/Users/someone/Code/widgets/work/widget-fix"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The root is taken as the operator typed it. `--root .` is the ordinary
// way to name an instance and every caller here joins the answer onto its
// own work rather than handing it to git from another directory, so unlike
// worktree.Resolve there is nothing to absolutize away from.
func TestPathKeepsARelativeRootRelative(t *testing.T) {
	if got := workdir.Path("instance", "widget-fix"); got != "instance/work/widget-fix" {
		t.Errorf("got %q, want %q", got, "instance/work/widget-fix")
	}
}
