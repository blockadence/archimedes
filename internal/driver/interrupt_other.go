//go:build !unix

package driver

import (
	"errors"
	"os/exec"
	"syscall"
)

// Everywhere else — Windows, in the platform list a release builds for —
// has neither half of what forwarding needs, so runs there behave exactly
// as they did before any of this existed: a signal stops archimedes, and
// the driver it started is on its own. Saying so here is better than a
// relay that catches an operator's interrupt and then cannot pass it on,
// which would leave them unable to stop a run at all.
//
// The drivers archimedes ships are bash and are not runnable there either,
// so this is the shape of a platform where the whole driver layer is
// somebody else's to supply.

const canForwardInterrupts = false

func isolateProcessGroup(*exec.Cmd) {}

func signalProcessGroup(int, syscall.Signal) error {
	return errors.New("this platform has no way to signal a process group")
}
