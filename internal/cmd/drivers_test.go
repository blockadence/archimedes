package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These drive the real command tree over the real shipped drivers, because
// the thing worth asserting is the one an operator acts on: what this
// install can actually run, and which of it is theirs.

func TestDriversListsTheOnesTheToolShips(t *testing.T) {
	root := t.TempDir()

	out := execute(t, "drivers", "--root", root)

	for _, name := range []string{"openspec", "pocock", "spec-kit"} {
		if !strings.Contains(out, name) {
			t.Errorf("`archimedes drivers` output does not list %s:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "built-in") {
		t.Errorf("output does not say the shipped drivers come from the tool:\n%s", out)
	}
}

func TestDriversNamesTheInstancesOwnDriversAsItsOwn(t *testing.T) {
	root := t.TempDir()
	stubDriverInstance(t, root, "homegrown", "mapped")

	out := execute(t, "drivers", "--root", root)

	if !strings.Contains(out, "homegrown") || !strings.Contains(out, "instance") {
		t.Errorf("output does not report homegrown as the instance's own:\n%s", out)
	}
}

// The diagnostic an instance created before the drivers moved into the tool
// needs: its copies still run, and no fix made to the shipped driver will
// ever reach them. Nothing removes them on the operator's behalf — the copy
// may be an edit they meant — so the report has to say it out loud.
func TestDriversFlagsAnInstanceCopyShadowingAShippedDriver(t *testing.T) {
	root := t.TempDir()
	stubDriverInstance(t, root, "openspec", "an old copy")

	out := execute(t, "drivers", "--root", root)

	if !strings.Contains(out, "shadow") {
		t.Errorf("output does not flag the instance's copy of openspec as shadowing the shipped one:\n%s", out)
	}
}

func TestDriversAdoptCopiesAShippedDriverIntoTheInstance(t *testing.T) {
	root := t.TempDir()

	out := execute(t, "drivers", "adopt", "--root", root, "openspec")

	dest := filepath.Join(root, "drivers", "openspec")
	if _, err := os.Stat(filepath.Join(dest, "run.sh")); err != nil {
		t.Fatalf("adopt did not leave a driver to edit at %s: %v", dest, err)
	}
	if !strings.Contains(out, dest) {
		t.Errorf("output does not say where the adopted driver landed:\n%s", out)
	}

	// And the listing now reports it as the instance's, shadowing ours.
	listed := execute(t, "drivers", "--root", root)
	if !strings.Contains(listed, "shadow") {
		t.Errorf("an adopted driver is not reported as shadowing the shipped one:\n%s", listed)
	}
}

func TestDriversAdoptRefusesANameTheToolDoesNotShip(t *testing.T) {
	err := executeErr(t, "drivers", "adopt", "--root", t.TempDir(), "nope")
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error = %v, want it to name the driver asked for", err)
	}
}
