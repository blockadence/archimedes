package version

import (
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
)

// A release build is the only one that can be certain what it is, so what it
// was told wins outright: the module system cannot contradict a tag someone
// deliberately cut.
func TestStampedVersionWins(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v0.0.1"}}

	if got := Resolve("v1.2.3", info); got != "v1.2.3" {
		t.Errorf("Resolve = %q, want the stamped v1.2.3", got)
	}
}

// `go install github.com/blockadence/gh-archimedes/cmd/archimedes@v1.2.3`
// links nothing, but the module system knows exactly which version it
// fetched.
func TestModuleVersionAnswersForAGoInstall(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}

	if got := Resolve("", info); got != "v1.2.3" {
		t.Errorf("Resolve = %q, want the module version v1.2.3", got)
	}
}

// What a plain `go build` from a checkout records since Go 1.24: a
// pseudo-version carrying the commit and, here, the marker for a tree with
// uncommitted edits. Passed through exactly as given -- it is assembled
// from the same stamps anything here would use, and it is already the
// better answer.
func TestACheckoutBuildsPseudoVersionIsPassedThroughIntact(t *testing.T) {
	const pseudo = "v0.0.0-20260907224930-f96985b1261c+dirty"
	info := &debug.BuildInfo{Main: debug.Module{Version: pseudo}}

	if got := Resolve("", info); got != pseudo {
		t.Errorf("Resolve = %q, want %q unchanged", got, pseudo)
	}
}

// "(devel)" is the module system saying it has no version to give. Reporting
// it verbatim would be reporting a placeholder as a version, which is the
// thing this package exists to stop.
func TestDevelPlaceholderIsNotAVersion(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}

	if got := Resolve("", info); got != "dev" {
		t.Errorf("Resolve = %q, want dev", got)
	}
}

// Built by a toolchain that recorded nothing -- -buildvcs off, or inside a
// git worktree, which Go does not take for a checkout. An empty version
// string would render as a missing one rather than an unknown one.
func TestNothingKnownStillReportsSomething(t *testing.T) {
	if got := Resolve("", nil); got != "dev" {
		t.Errorf("Resolve = %q, want dev", got)
	}
}

// Whatever route built the binary running this test, the answer has to be
// usable in the sentence "which build is this?" -- never empty, and never
// the module system's placeholder leaking through.
func TestCurrentAlwaysAnswers(t *testing.T) {
	got := Current()

	if got == "" {
		t.Fatal(`Current() = "", want some version`)
	}
	if strings.Contains(got, "devel") {
		t.Errorf("Current() = %q, want a version rather than the module system's placeholder", got)
	}
	if !regexp.MustCompile(`^(dev|v\d)`).MatchString(got) {
		t.Errorf("Current() = %q, want either a v-version or the dev marker", got)
	}
}
