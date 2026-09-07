// Package version answers the first question anyone asks about a build that
// misbehaves: which build is this?
//
// Three routes produce a binary and each leaves behind a different trace, so
// the answer is assembled from whichever of them ran:
//
//   - A release build links the tag in directly (see stamped below). This is
//     the route that matters most, because a downloaded release artifact has
//     no checkout beside it to look at -- it is the only evidence there is.
//   - `go install <module>/cmd/archimedes@v1.2.3` links nothing, but the
//     module system records which version it fetched.
//   - A build from a working tree has neither, and the Go toolchain stamps
//     the commit it was built from instead.
//
// Kept out of internal/cmd deliberately: this is a decision with rules worth
// testing on their own, not CLI wiring, and the linker needs a stable
// package path to aim -X at.
package version

import (
	"runtime/debug"
	"strings"
)

// stamped is empty in every build but a release one, where the linker fills
// it in with the tag being released:
//
//	go build -ldflags="-X github.com/blockadence/gh-archimedes/internal/version.stamped=v1.2.3" ./cmd/archimedes
//
// .github/release-build.sh is the only thing that passes it, and it is the
// reason this lives in a package of its own: -X needs a path to aim at that
// does not move every time the CLI wiring is rearranged.
var stamped string

// shaLen is git's own abbreviation, so a version reads like something you
// can paste back into `git show`.
const shaLen = 7

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
	return fromCheckout(info)
}

// moduleVersion is the version the module system fetched, or "" when there
// wasn't one. "(devel)" is the module system saying it has no version to
// give -- every build from a working tree says it -- and passing that on
// would be reporting a placeholder as a version, which is the whole thing
// this package exists to stop.
func moduleVersion(info *debug.BuildInfo) string {
	if info == nil {
		return ""
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return ""
}

// fromCheckout describes a build made from a working tree: not a version,
// and it does not pretend to be one, but the commit is enough to find the
// source. A tree with uncommitted edits matches no commit at all, so it
// says so rather than naming one it isn't.
//
// The Go toolchain only stamps this when it recognises the directory as a
// checkout, and it looks for a .git *directory* -- so a build made inside a
// git worktree gets a bare "dev". Worth knowing, since this project is
// worked on in worktrees, but not worth working around: neither route that
// installs the tool comes through here.
func fromCheckout(info *debug.BuildInfo) string {
	rev := setting(info, "vcs.revision")
	if rev == "" {
		return "dev"
	}
	if len(rev) > shaLen {
		rev = rev[:shaLen]
	}
	if setting(info, "vcs.modified") == "true" {
		return "dev (" + rev + ", dirty)"
	}
	return "dev (" + rev + ")"
}

func setting(info *debug.BuildInfo, key string) string {
	if info == nil {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == key {
			return strings.TrimSpace(s.Value)
		}
	}
	return ""
}
