package driver_test

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/blockadence/gh-archimedes/internal/driver"
)

// A Set is two layers, and what these tests are about is which of them
// answers a given name — the question the instance/tool ownership split
// turns on. The layers are stand-ins here (a temp dir, a MapFS) rather than
// the real ones, so a test says which layer won without depending on what
// this repo happens to ship today.

// builtins is a stand-in for the drivers the binary carries: each one
// records which layer it came from into the map it writes. It carries both
// kinds of helper a driver's command can source — one of its own, beside
// the command, and one out of the shared lib/ that sits alongside every
// driver rather than inside any of them.
func builtins() fstest.MapFS {
	return fstest.MapFS{
		"shipped/driver.yaml": &fstest.MapFile{Data: []byte("name: shipped\ndescription: one the tool ships\noutput_mode: path-parameterized\ncommand: run.sh\n")},
		"shipped/run.sh":      &fstest.MapFile{Data: []byte("#!/usr/bin/env bash\nset -euo pipefail\nhere=\"$(dirname \"${BASH_SOURCE[0]}\")\"\n. \"$here/lib.sh\"\n. \"$here/../lib/shared.sh\"\necho \"built-in shipped, $(helper), $(shared_helper)\" > \"$2\"\n")},
		"shipped/lib.sh":      &fstest.MapFile{Data: []byte("helper() { echo 'sourced helper ran'; }\n")},
		"lib/shared.sh":       &fstest.MapFile{Data: []byte("shared_helper() { echo 'shared helper ran'; }\n")},
	}
}

func TestABuiltinDriverRunsWhenTheInstanceHasNoneOfItsOwn(t *testing.T) {
	set := driver.Set{Dir: t.TempDir(), Builtin: builtins()}
	out := filepath.Join(t.TempDir(), "CONTEXT.md")

	if err := set.Run("shipped", repoDir(t), out, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := readFile(t, out)
	if !strings.Contains(got, "built-in shipped") {
		t.Errorf("output = %q, want the built-in driver to have run", got)
	}
	// A driver is a directory, not a file: the helper its command sources
	// has to come along with it, or the built-in layer only works for the
	// drivers simple enough to be one script.
	if !strings.Contains(got, "sourced helper ran") {
		t.Errorf("output = %q, want the built-in's sourced helper to have been carried with it", got)
	}
}

// Two drivers that both have to leave someone else's repository exactly as
// they found it should not each carry their own copy of the code that does
// it — the second copy is the one that drifts, and it drifts on the
// failure path where nobody is watching. So the shared helpers live beside
// the drivers rather than inside one of them, and resolving any driver has
// to bring them along: `../lib/` has to be there whichever layer answered.
func TestABuiltinDriverGetsTheSharedHelpersBesideIt(t *testing.T) {
	set := driver.Set{Dir: t.TempDir(), Builtin: builtins()}
	out := filepath.Join(t.TempDir(), "CONTEXT.md")

	if err := set.Run("shipped", repoDir(t), out, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := readFile(t, out); !strings.Contains(got, "shared helper ran") {
		t.Errorf("output = %q, want the shared lib/ to have been unpacked alongside the driver", got)
	}
}

// lib/ is not a driver and must never read as one: it declares no manifest,
// so the listing passes over it exactly as it passes over any other
// directory under drivers/ that declares nothing, and naming it is an
// unknown driver rather than a confusing failure inside it.
func TestTheSharedHelperDirectoryIsNotADriver(t *testing.T) {
	set := driver.Set{Dir: t.TempDir(), Builtin: builtins()}

	got, err := set.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range got {
		if e.Name == "lib" {
			t.Errorf("List() = %+v, want the shared helper directory left out of the listing", got)
		}
	}

	err = set.Run("lib", repoDir(t), filepath.Join(t.TempDir(), "CONTEXT.md"), io.Discard)
	if err == nil {
		t.Fatal("expected naming the shared helper directory to be an unknown driver, got nil")
	}
	if !strings.Contains(err.Error(), "unknown driver") {
		t.Errorf("error %q, want it to read as an unknown driver", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// The criterion the whole ownership answer turns on from the operator's
// side: a driver they wrote — or took over — is theirs, and upgrading the
// tool never displaces it.
func TestAnInstancesOwnDriverWinsOverTheOneTheToolShips(t *testing.T) {
	dir := t.TempDir()
	writeDriver(t, dir, "shipped", "name: shipped\noutput_mode: path-parameterized\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nset -euo pipefail\necho 'the instance edited this one' > \"$2\"\n")
	set := driver.Set{Dir: dir, Builtin: builtins()}
	out := filepath.Join(t.TempDir(), "CONTEXT.md")

	if err := set.Run("shipped", repoDir(t), out, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := readFile(t, out); !strings.Contains(got, "the instance edited this one") {
		t.Errorf("output = %q, want the instance's own copy to have run", got)
	}
}

// A name neither layer answers is a misconfiguration, and the error has to
// say where it looked — "unknown driver" is unhelpful advice when there are
// two places it could have been.
func TestANameNeitherLayerAnswersNamesBothPlacesItLooked(t *testing.T) {
	dir := t.TempDir()
	set := driver.Set{Dir: dir, Builtin: builtins()}

	err := set.Run("nowhere", repoDir(t), filepath.Join(t.TempDir(), "CONTEXT.md"), io.Discard)
	if err == nil {
		t.Fatal("expected an error for a driver neither layer supplies, got nil")
	}
	for _, want := range []string{"nowhere", dir, "archimedes"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// Unpacking a built-in is an implementation detail of running one, and it
// has to stay one: a driver run that left a copy of the driver behind in
// the temp directory would accumulate one per mapped repo, and the stale
// copies would be the ones an upgrade fails to reach.
func TestRunningABuiltinLeavesNoUnpackedCopyBehind(t *testing.T) {
	// A temp directory of this test's own, so what is counted is what this
	// run unpacked rather than whatever else the machine has in /tmp.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	set := driver.Set{Dir: t.TempDir(), Builtin: builtins()}

	if err := set.Run("shipped", repoDir(t), filepath.Join(t.TempDir(), "CONTEXT.md"), io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}

	left, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("the run left %d entries behind in the temp directory: %v", len(left), left)
	}
}

// The criterion this whole arrangement exists for, from the tool's side: an
// instance that has not changed at all runs the fixed driver once the
// binary carrying it is upgraded. Two built-in layers over one unchanged
// instance is exactly that upgrade, with nothing else moving.
func TestUpgradingTheToolDeliversAFixToAnUnchangedInstance(t *testing.T) {
	instance := t.TempDir()
	repo, out := repoDir(t), filepath.Join(t.TempDir(), "CONTEXT.md")

	before := builtins()
	if err := (driver.Set{Dir: instance, Builtin: before}).Run("shipped", repo, out, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, out); strings.Contains(got, "the bug is fixed") {
		t.Fatalf("output = %q before the fix was made", got)
	}

	after := builtins()
	after["shipped/run.sh"] = &fstest.MapFile{Data: []byte(
		"#!/usr/bin/env bash\nset -euo pipefail\necho 'the bug is fixed' > \"$2\"\n")}

	if err := (driver.Set{Dir: instance, Builtin: after}).Run("shipped", repo, out, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, out); !strings.Contains(got, "the bug is fixed") {
		t.Errorf("output = %q, want the fixed driver to have run", got)
	}

	// And the delivery cost the instance nothing: no copy arrived in it to
	// go stale next time.
	left, err := os.ReadDir(instance)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("running shipped drivers put %v into the instance", left)
	}
}

// Listing is the command an operator reaches for when something is wrong,
// so one driver that cannot be read must not cost them the report on the
// rest.
func TestListReportsADriverThatWillNotLoadRatherThanFailing(t *testing.T) {
	dir := t.TempDir()
	writeDriver(t, dir, "broken", "name: [unterminated\n", "#!/usr/bin/env bash\n")
	set := driver.Set{Dir: dir, Builtin: builtins()}

	got, err := set.List()
	if err != nil {
		t.Fatalf("List failed outright over one unreadable manifest: %v", err)
	}

	var broken driver.Entry
	for _, e := range got {
		if e.Name == "broken" {
			broken = e
		}
	}
	if broken.Err == nil {
		t.Errorf("List() = %+v, want the unreadable driver carrying its own error", got)
	}
	if len(got) != 2 {
		t.Errorf("List() = %+v, want the shipped driver reported alongside it", got)
	}
}

// What an operator needs to be told is not just which names resolve but
// which layer answers each, because the two differ in who maintains it.
func TestListReportsWhichLayerAnswersEachName(t *testing.T) {
	dir := t.TempDir()
	writeDriver(t, dir, "homegrown", "name: homegrown\ndescription: written here\noutput_mode: path-parameterized\ncommand: run.sh\n", "#!/usr/bin/env bash\n")
	// A directory under drivers/ that declares no driver is not one, and
	// listing is no place to complain about it.
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	set := driver.Set{Dir: dir, Builtin: builtins()}

	got, err := set.List()
	if err != nil {
		t.Fatal(err)
	}

	want := []driver.Entry{
		{Name: "homegrown", Description: "written here", Origin: driver.FromInstance},
		{Name: "shipped", Description: "one the tool ships", Origin: driver.FromBuiltin},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() = %+v, want %+v", got, want)
	}
}

// The case this exists for: an instance created before the drivers moved
// into the tool still holds copies of them, and those copies go on running
// while every fix made here passes them by. Nothing can safely delete them
// — an operator may have edited one — so the report says so instead.
func TestListFlagsAnInstanceCopyShadowingAShippedDriver(t *testing.T) {
	dir := t.TempDir()
	writeDriver(t, dir, "shipped", "name: shipped\ndescription: a copy from before\noutput_mode: path-parameterized\ncommand: run.sh\n", "#!/usr/bin/env bash\n")
	set := driver.Set{Dir: dir, Builtin: builtins()}

	got, err := set.List()
	if err != nil {
		t.Fatal(err)
	}

	want := []driver.Entry{
		{Name: "shipped", Description: "a copy from before", Origin: driver.FromInstance, Shadows: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() = %+v, want the instance's copy reported as shadowing the shipped one: %+v", got, want)
	}
}

// An instance whose drivers/ was never created at all is the normal case
// for a fresh one, not an error: everything it can run comes from the tool.
func TestListWorksForAnInstanceWithNoDriversDirectoryAtAll(t *testing.T) {
	set := driver.Set{Dir: filepath.Join(t.TempDir(), "never-created"), Builtin: builtins()}

	got, err := set.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "shipped" {
		t.Errorf("List() = %+v, want just the shipped driver", got)
	}
}

// Taking a shipped driver over has to be an explicit act, and it has to
// leave the operator with the real thing to edit — the whole directory,
// runnable, not a name they now have to write 200 lines of bash behind.
func TestAdoptCopiesAShippedDriverIntoTheInstanceWhereItThenWins(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "drivers")
	set := driver.Set{Dir: dir, Builtin: builtins()}

	got, err := set.Adopt("shipped")
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	if want := filepath.Join(dir, "shipped"); got != want {
		t.Errorf("Adopt returned %q, want %q", got, want)
	}
	// The sourced helper comes too, and only the command is runnable.
	assertMode(t, filepath.Join(got, "run.sh"), true)
	assertMode(t, filepath.Join(got, "lib.sh"), false)

	// The point of adopting: edit it, and the edit is what runs.
	if err := os.WriteFile(filepath.Join(got, "run.sh"),
		[]byte("#!/usr/bin/env bash\nset -euo pipefail\necho 'edited after adopting' > \"$2\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "CONTEXT.md")
	if err := set.Run("shipped", repoDir(t), out, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, out); !strings.Contains(got, "edited after adopting") {
		t.Errorf("output = %q, want the adopted copy to have run", got)
	}
}

// Adopting hands over the whole of what the driver needs to run, and the
// shared helpers are part of that: an adopted driver whose `../lib/` was
// left behind in the binary would fail only once it was already running
// inside somebody's repository.
func TestAdoptBringsTheSharedHelpersTheAdoptedDriverSources(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "drivers")
	set := driver.Set{Dir: dir, Builtin: builtins()}

	if _, err := set.Adopt("shipped"); err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	// Sourced, not run — the same rule the driver's own helper follows.
	assertMode(t, filepath.Join(dir, "lib", "shared.sh"), false)

	out := filepath.Join(t.TempDir(), "CONTEXT.md")
	if err := set.Run("shipped", repoDir(t), out, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, out); !strings.Contains(got, "shared helper ran") {
		t.Errorf("output = %q, want the adopted driver to find the shared helpers it sources", got)
	}
}

// The shared helpers an instance already has are the instance's, by the
// same rule its drivers are: adopting a second driver that sources them
// must not quietly replace the copy the operator may have edited.
func TestAdoptLeavesSharedHelpersTheInstanceAlreadyHasAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "drivers")
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(dir, "lib", "shared.sh")
	if err := os.WriteFile(mine, []byte("shared_helper() { echo 'mine, edited'; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	set := driver.Set{Dir: dir, Builtin: builtins()}

	if _, err := set.Adopt("shipped"); err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	if got := readFile(t, mine); !strings.Contains(got, "mine, edited") {
		t.Errorf("the instance's own shared helper was overwritten: %q", got)
	}
}

// Overwriting is the one thing adopting must never do: the copy already
// there is the operator's, and it may be the very edit they are about to
// lose.
func TestAdoptRefusesToOverwriteADriverTheInstanceAlreadyHas(t *testing.T) {
	dir := t.TempDir()
	writeDriver(t, dir, "shipped", "name: shipped\noutput_mode: path-parameterized\ncommand: run.sh\n",
		"#!/usr/bin/env bash\necho mine\n")
	set := driver.Set{Dir: dir, Builtin: builtins()}

	if _, err := set.Adopt("shipped"); err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got := readFile(t, filepath.Join(dir, "shipped", "run.sh")); !strings.Contains(got, "mine") {
		t.Errorf("the instance's own copy was overwritten: %q", got)
	}
}

func TestAdoptRefusesANameTheToolDoesNotShip(t *testing.T) {
	set := driver.Set{Dir: t.TempDir(), Builtin: builtins()}

	_, err := set.Adopt("nowhere")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "nowhere") {
		t.Errorf("error %q does not name the driver asked for", err)
	}
}

func assertMode(t *testing.T, path string, executable bool) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode()&0o111 != 0; got != executable {
		t.Errorf("%s executable = %v, want %v (mode %v)", path, got, executable, info.Mode())
	}
}
