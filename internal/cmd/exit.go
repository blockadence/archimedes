package cmd

import (
	"github.com/blockadence/gh-archimedes/internal/driver"
)

// ExitStatus is what archimedes leaves behind for whoever ran it, given
// whatever Execute came back with.
//
// One is nearly always the answer: a failure is a failure, and the message
// fang has already printed is where the detail is. The exception is a run a
// signal stopped, which exits with the status the driver chose — 130 for a
// SIGINT, 143 for a SIGTERM, by the convention the drivers' rollback
// follows. Passing that through is more use to a supervisor, a `timeout`,
// or a parent harness than a status archimedes invented, and it is the only
// thing a caller has to read to find out whether the run it stopped got to
// put the target repo back.
//
// Here rather than in main so that the mapping from an error to a status is
// beside the command tree that produces those errors, and testable without
// a process.
func ExitStatus(err error) int {
	if err == nil {
		return 0
	}
	if status, ok := driver.ExitStatus(err); ok {
		return status
	}
	return 1
}
