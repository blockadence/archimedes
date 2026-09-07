package version

import (
	"runtime/debug"
	"testing"
)

// A release build is the only one that can be sure what it is, so what it
// was told wins outright: neither of the other two sources can contradict a
// tag someone deliberately cut.
func TestStampedVersionWins(t *testing.T) {
	info := &debug.BuildInfo{
		Main:     debug.Module{Version: "v0.0.1"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "0123456789abcdef"}},
	}

	if got := Resolve("v1.2.3", info); got != "v1.2.3" {
		t.Errorf("Resolve = %q, want the stamped v1.2.3", got)
	}
}

// `go install github.com/blockadence/gh-archimedes/cmd/archimedes@v1.2.3`
// links nothing, but the module system knows exactly which version it
// fetched. That is a real answer and belongs ahead of any guess.
func TestModuleVersionAnswersForAGoInstall(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}

	if got := Resolve("", info); got != "v1.2.3" {
		t.Errorf("Resolve = %q, want the module version v1.2.3", got)
	}
}

// "(devel)" is what the module system says when it has no version to give,
// which is every build made from a working tree. Reporting it verbatim
// would be reporting a placeholder as a version -- the thing this exists to
// stop -- so it falls through to what the checkout can say for itself.
func TestDevelPlaceholderFallsThroughToTheCommit(t *testing.T) {
	info := &debug.BuildInfo{
		Main:     debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "0123456789abcdef"}},
	}

	if got := Resolve("", info); got != "dev (0123456)" {
		t.Errorf("Resolve = %q, want dev (0123456)", got)
	}
}

// A tree with uncommitted edits builds a binary that matches no commit at
// all, so naming the commit without saying so would be the one genuinely
// misleading answer available.
func TestADirtyTreeSaysSo(t *testing.T) {
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef"},
			{Key: "vcs.modified", Value: "true"},
		},
	}

	if got := Resolve("", info); got != "dev (0123456, dirty)" {
		t.Errorf("Resolve = %q, want dev (0123456, dirty)", got)
	}
}

func TestACleanTreeOmitsTheDirtyMarker(t *testing.T) {
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef"},
			{Key: "vcs.modified", Value: "false"},
		},
	}

	if got := Resolve("", info); got != "dev (0123456)" {
		t.Errorf("Resolve = %q, want dev (0123456)", got)
	}
}

// Built outside a checkout, or by a toolchain that stamped nothing: there
// is nothing truthful left to add, and an empty version string would render
// as a missing one rather than an unknown one.
func TestNothingKnownStillReportsSomething(t *testing.T) {
	for name, info := range map[string]*debug.BuildInfo{
		"no build info at all": nil,
		"build info with no vcs stamps": {
			Main: debug.Module{Version: "(devel)"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Resolve("", info); got != "dev" {
				t.Errorf("Resolve = %q, want dev", got)
			}
		})
	}
}

// Whatever route built the binary running this test, the answer it gives
// has to be usable in the sentence "which build is this?" -- never empty,
// and never the bare package placeholder.
func TestCurrentAlwaysAnswers(t *testing.T) {
	if got := Current(); got == "" {
		t.Error("Current() = \"\", want some version")
	}
}
