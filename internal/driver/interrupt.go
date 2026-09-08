package driver

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// An interrupt aimed at archimedes has to reach the driver archimedes is
// waiting on, and archimedes has to still be there when the driver answers.
//
// Go forwards nothing to a child process. So without this, `kill -TERM` on
// the archimedes pid stops archimedes and leaves the driver running: the
// operator has their prompt back, a third-party toolchain scaffolded into
// their repository, and nothing watching the process that was going to take
// it out again. The driver does get there eventually, when its session ends
// on its own, but a `claude -p` session runs for minutes and by then nobody
// is reading.
//
// The half this closes is the cheap half, and it is worth being exact about
// which one. Forwarding only helps a driver still alive to receive it: a
// driver that was SIGKILL'd, or that died with the machine, still needs the
// runner that holds the snapshot itself which drivers/lib/repo-snapshot.sh
// asks for. What it does buy is that the rollback those drivers already
// have stops being something that fires under a terminal and starts being
// something that fires whenever a run is stopped.
//
// Forwarding without waiting would be today's behaviour with extra steps.
// The rollback happens in the driver's exit trap, after its session
// returns, so archimedes passes the signal on and then goes back to waiting
// — which is also what lets it relay what the driver said about the repo,
// and exit with the status the driver chose.

// forwarded are the signals archimedes passes on. SIGKILL is deliberately
// absent, and could not be here in any case: it can be neither caught to
// forward nor trapped to act on, which is exactly why a run stopped that
// way is the case repo-snapshot.sh names as beyond a driver's reach.
var forwarded = []os.Signal{os.Interrupt, syscall.SIGTERM}

// Stopped is a driver run that ended because archimedes was signalled and
// passed the signal on. It is a distinct error rather than a message
// because something acts on it: it is what archimedes exits with (see
// ExitStatus), and a caller asking whether the target repo was left clean
// has nothing else to read.
type Stopped struct {
	// Signal is what archimedes received and forwarded.
	Signal syscall.Signal
	// Status is what archimedes should exit with — see stoppedBy.
	Status int
}

func (e *Stopped) Error() string {
	// "the run was interrupted" rather than "the driver was told": a
	// signal landing as the driver was already on its way out is still
	// what stopped this run and is still recorded (see relay.release),
	// but it was never passed on to anything.
	return fmt.Sprintf("stopped by %s: the run was interrupted and the driver exited %d — what it did with the repo is in its own output above",
		signalName(e.Signal), e.Status)
}

// StoppedStatus reports what archimedes should exit with after err, and
// whether err is a run a signal stopped at all. Anything else is an
// ordinary failure and exits 1 like every other one.
//
// Named for the case it answers rather than for what the caller does with
// it, because cmd.ExitStatus is the one that answers "what does archimedes
// exit with" — over every error, not only these — and two functions called
// ExitStatus with different contracts would be one confusion too many at
// the seam between them.
func StoppedStatus(err error) (int, bool) {
	var stopped *Stopped
	if errors.As(err, &stopped) {
		return stopped.Status, true
	}
	return 0, false
}

// stoppedBy is the error a run sig ended reports, given how the driver
// finished — or a nil state for one that never got started.
//
// The drivers' convention is 128 + the signal's number — 130 for SIGINT,
// 143 for SIGTERM — and passing the driver's own status straight through is
// more use than one archimedes invented. A driver killed by the signal
// rather than trapping it reports no status of its own, and one that exited
// zero after being told to stop did not thereby succeed; both get the
// convention's answer instead.
func stoppedBy(sig syscall.Signal, state *os.ProcessState) *Stopped {
	status := 128 + int(sig)
	if state != nil {
		if code := state.ExitCode(); code > 0 {
			status = code
		}
	}
	return &Stopped{Signal: sig, Status: status}
}

// signalName is what an operator calls the signal. syscall.Signal's own
// String is prose ("interrupt", "terminated"), which reads oddly beside the
// SIGINT in a driver's own message and the 130 it exits with.
func signalName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	default:
		return sig.String()
	}
}

// relay carries an interrupt archimedes received to the driver's process
// group, and remembers that it did so the run can say what stopped it.
type relay struct {
	progress io.Writer
	// ch is nil on a platform that cannot forward, which is what every
	// method here reads to mean "do nothing".
	ch chan os.Signal
	// done ends the goroutine; finished reports that it has, so the
	// signal it recorded is read by one thing at a time.
	done     chan struct{}
	finished chan struct{}
	// first is the signal that was forwarded. Everything after it is
	// answered with a line and nothing else — see forward.
	first os.Signal
}

// watchForInterrupts starts catching the forwardable signals. It is called
// before the driver is started, not after, for two reasons: a signal
// arriving in the window before there is a pid to forward to waits in the
// buffer rather than being dropped, and — the one that matters more — it
// does not take archimedes down on the spot while a driver is already
// unpacking itself into somebody's repository.
//
// Nothing outside a driver run is covered. An archimedes that is not
// waiting on a driver has nothing to forward to and nothing to wait for, so
// Ctrl-C there should stop it immediately, the way it always did.
//
// What buffering cannot do is make the driver ready to hear it. A signal
// forwarded before the driver has armed its own traps is one the driver
// takes the default action on, and the run ends with no rollback — the
// same shape as the ignored-at-start-up case repo-snapshot.sh names, and
// as unreachable from here, since there is nothing to forward to before
// there is a process. What bounds it is a driver-side ordering rather than
// anything arranged here: a driver that arms its traps before it lets
// anything write to the target repo has nothing in there to roll back
// during that window. The two shipped drivers that put a repo back do,
// and drivers/README.md asks it of the next one.
func watchForInterrupts(progress io.Writer) *relay {
	r := &relay{progress: progress}
	if !canForwardInterrupts {
		return r
	}
	r.ch = make(chan os.Signal, len(forwarded))
	signal.Notify(r.ch, forwarded...)
	return r
}

// forwardTo starts passing what arrives on to the process group led by pid.
func (r *relay) forwardTo(pid int) {
	if r.ch == nil {
		return
	}
	r.done = make(chan struct{})
	r.finished = make(chan struct{})
	go func() {
		defer close(r.finished)
		for {
			select {
			case sig := <-r.ch:
				r.forward(sig, pid)
			case <-r.done:
				return
			}
		}
	}()
}

// forward passes the first signal to the driver, and answers every one
// after it with a line and nothing else.
//
// Only the first, because the second signal is an operator hitting Ctrl-C
// again after the first appeared to do nothing — and a driver is at its
// most fragile precisely then. Its rollback is already running, and a
// signal that lands partway through leaves what has been undone undone and
// the rest not, which repo-snapshot.sh names as the state it cannot get a
// repo out of. Passing the second copy on would make that easier to reach,
// not harder, so the operator is told what is being waited for instead.
func (r *relay) forward(sig os.Signal, pid int) {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return
	}
	if r.first != nil {
		fmt.Fprintf(r.progress, "%s again — the driver has already been told to stop; still waiting for it to put the repo back. Passing this on would interrupt that halfway and leave the repo between two states, so it is not passed on: only SIGKILL ends this now, and it would strand whatever the run has left in there.\n", signalName(s))
		return
	}
	fmt.Fprintf(r.progress, "%s — telling the driver to stop, and waiting for it to put the repo back\n", signalName(s))
	if err := signalProcessGroup(pid, s); err != nil {
		// Not recorded as forwarded, and the relay stands down. An
		// archimedes holding an interrupt it could not pass on would
		// go on swallowing every one after it too — the operator
		// unable to stop the run at all, which is the outcome
		// interrupt_other.go declines to build on a platform that
		// cannot forward, and no better arrived at by accident here.
		// Standing down puts the next signal back where it was before
		// any of this: it stops archimedes, and the driver is on its
		// own, which is worse than a rollback and better than nothing
		// the operator can do.
		fmt.Fprintf(r.progress, "could not reach the driver with %s: %v — another interrupt will stop archimedes itself, leaving the driver to finish unwatched\n", signalName(s), err)
		signal.Stop(r.ch)
		return
	}
	r.first = sig
}

// release stands the relay down, once the driver has been waited for, and
// reports the signal that stopped the run if one did.
func (r *relay) release() (syscall.Signal, bool) {
	if r.ch == nil {
		return 0, false
	}
	// In this order: nothing more can be delivered once Stop returns, then
	// the goroutine is ended and waited for, and only then is what it
	// recorded read — so first has one writer at a time without a lock
	// standing between a signal and the driver it is bound for.
	signal.Stop(r.ch)
	if r.finished != nil {
		close(r.done)
		<-r.finished
	}
	// Anything that arrived but was never looked at still stopped this
	// run, even if it landed as the driver was already on its way out.
	// Dropping it would report an ordinary failure for a run an operator
	// interrupted.
	for {
		select {
		case sig := <-r.ch:
			if r.first == nil {
				r.first = sig
			}
		default:
			s, ok := r.first.(syscall.Signal)
			return s, ok
		}
	}
}

// serialized guards progress with a lock, because two things write to it
// while a driver runs: the copier os/exec runs over the driver's own
// streams, and the relay's lines about the signal it forwarded.
//
// Handing the same guarded value to both Stdout and Stderr keeps os/exec
// down to a single copier for the pair, so the driver's two streams stay
// interleaved in the order it wrote them, as they were before this.
type serialized struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *serialized) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
