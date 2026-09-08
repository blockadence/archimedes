//go:build unix

package driver

import (
	"os/exec"
	"syscall"
)

// Unix has both halves of what forwarding needs: a way to give the driver
// a process group of its own, and a way to signal that group by name.

// canForwardInterrupts says this platform can do both, so a run arms the
// relay in interrupt.go.
const canForwardInterrupts = true

// isolateProcessGroup puts the driver — and everything it goes on to start
// — in a process group of its own.
//
// Without it the driver stays in archimedes' group, which is what makes
// Ctrl-C at a terminal work today: the terminal signals the whole
// foreground group, so the driver gets a copy nobody arranged for it. That
// is worth losing. It only holds for a terminal, so the same run stopped by
// `kill` on a pid, a supervisor or a `timeout(1)` leaves the driver
// untouched — and where it does hold, the driver's own copy arrives
// alongside the one archimedes forwards, so an operator's two Ctrl-Cs
// become four signals aimed at a rollback that only survives one.
//
// A group of its own makes what the driver receives archimedes' to decide,
// in both directions: it receives what is forwarded, and nothing else.
func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalProcessGroup delivers sig to the group led by pid — which is the
// driver's, since Setpgid with no Pgid makes the child its own leader.
//
// The group rather than the driver alone, because the driver is a shell
// blocked on an agent session it started: a signal that reaches only the
// shell is deferred until that session returns on its own, which for a
// `claude -p` run is the minutes this is trying not to wait for.
func signalProcessGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}
