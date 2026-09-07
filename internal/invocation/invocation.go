// Package invocation answers what to call this program in anything an
// operator reads back: usage lines, the error that tells them to run
// something else, the command a notification says to run about a condition.
//
// It cannot be a constant, because there are two installs and they are
// typed differently. Standalone the binary is `archimedes`; installed as a
// gh extension the same binary is reached as `gh archimedes`, and an
// operator who typed that and is told to run `archimedes bootstrap` has
// been handed a command they have not got. gh announces itself by setting
// GH_EXTENSION=1, documented in `gh help environment` for exactly this.
//
// The line for callers is whether an operator could copy the string and run
// it. Two kinds of mention deliberately stay fixed:
//
//   - Prose naming the program as a thing rather than as something to type
//     -- "the drivers archimedes ships" -- which is the same program under
//     either install.
//   - Anything written into a file. A HOUSE_RULES.md or a PR template this
//     tool commits into somebody's repository has to read the same whoever
//     generated it; keying its content to how the generating operator
//     happened to install would put a spurious diff in every such repo the
//     first time someone with the other install ran the sync.
package invocation

import "os"

// Name is the command an operator types to reach this program.
func Name() string {
	if os.Getenv("GH_EXTENSION") == "" {
		return "archimedes"
	}
	return "gh archimedes"
}
