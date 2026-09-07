// Package version answers the first question anyone asks about a build that
// misbehaves: which build is this?
//
// Three things can produce the answer, most authoritative first:
//
//   - A release build links the tag in (see stamped below). This is the one
//     that matters most, because a downloaded release artifact has no
//     checkout beside it -- what the binary says is the only evidence there
//     is.
//   - Otherwise the module system answers for itself, and since Go 1.24 it
//     answers for far more than it used to: `go install <module>@v1.2.3`
//     reports that version, and a plain `go build` from a checkout reports
//     a pseudo-version synthesized from the commit, carrying the sha and a
//     +dirty marker when the tree had uncommitted edits. It is passed
//     through exactly as given -- it is already the better answer than
//     anything reassembled here from the same stamps.
//   - Failing both, "dev", because there is nothing truthful left to add.
//     That is a build whose toolchain recorded nothing at all: -buildvcs is
//     off, or the build happened inside a git worktree, which Go does not
//     recognise as a checkout (it looks for a .git *directory*, and a
//     worktree's is a file). Worth knowing, since this project is worked on
//     in worktrees -- but not worth working around, because neither route
//     that installs the tool comes through here.
//
// Kept out of internal/cmd deliberately: this is a decision with rules
// worth testing on their own, not CLI wiring, and the linker needs a stable
// package path to aim -X at.
package version

import "runtime/debug"

// stamped is empty in every build but a release one, where the linker fills
// it in with the tag being released:
//
//	go build -ldflags="-X github.com/blockadence/gh-archimedes/internal/version.stamped=v1.2.3" ./cmd/archimedes
//
// .github/release-build.sh is the only thing that passes it, and it is the
// reason this lives in a package of its own: -X needs a path to aim at that
// does not move every time the CLI wiring is rearranged.
var stamped string

// Current reports the version of the running binary.
func Current() string {
	// ReadBuildInfo already answers nil when it has nothing, which is the
	// case Resolve takes.
	info, _ := debug.ReadBuildInfo()
	return Resolve(stamped, info)
}

// Resolve picks the most authoritative answer the build left behind.
// linked is what the linker stamped in, empty in everything but a release
// build; info may be nil, for a binary whose toolchain recorded nothing at
// all.
func Resolve(linked string, info *debug.BuildInfo) string {
	if linked != "" {
		return linked
	}
	if v := moduleVersion(info); v != "" {
		return v
	}
	return "dev"
}

// moduleVersion is the version the module system recorded, or "" when there
// wasn't one. "(devel)" is the module system saying it has nothing to give,
// and passing that on would be reporting a placeholder as a version --
// which is the whole thing this package exists to stop.
func moduleVersion(info *debug.BuildInfo) string {
	if info == nil {
		return ""
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return ""
}
