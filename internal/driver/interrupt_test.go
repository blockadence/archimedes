//go:build unix

package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/blockadence/gh-archimedes/internal/driver"
)

// The stub driver these tests interrupt. It does what the shipped
// fixed-location drivers do, in miniature: puts something in the target
// repo, arms a trap that takes an interrupt into a rollback, and then sits
// in a session it cannot be hurried out of.
//
// Everything it does, it does in the repo it was pointed at, so a test
// reads its progress from files rather than from timing: STARTED says the
// trap is armed and the scaffolding is really in there, SIGNALS records
// every signal that reached it, and SCAFFOLD/ is what the rollback has to
// take away again.
//
// The rollback sleeps, deliberately. A rollback that returned instantly
// would pass a test that forwarded the signal and exited immediately
// afterwards — which is the behaviour being fixed, with extra steps — and
// it would leave no window for the second signal the relay must not pass
// on.
const interruptibleDriver = `#!/usr/bin/env bash
set -uo pipefail
repo="$1"
mkdir -p "$repo/SCAFFOLD"
echo scaffolded > "$repo/SCAFFOLD/thing.txt"

rollback() { # <signal-name> <status>
  echo "$1" >> "$repo/SIGNALS"
  sleep 1
  rm -rf "$repo/SCAFFOLD"
  echo "interrupted by SIG$1 -- rolled $repo back to how it was found" >&2
  exit "$2"
}
trap 'rollback INT 130' INT
trap 'rollback TERM 143' TERM

touch "$repo/STARTED"
waited=0
while [ "$waited" -lt 300 ]; do sleep 0.1; waited=$((waited + 1)); done
`

// interruptible installs that driver as an instance's own and returns the
// Set holding it, beside the repo it will be pointed at.
func interruptible(t *testing.T) (driver.Set, string) {
	t.Helper()
	dir := t.TempDir()
	writeDriver(t, dir, "stub-interruptible",
		"name: stub-interruptible\noutput_mode: path-parameterized\ncommand: run.sh\n",
		interruptibleDriver)
	return driver.Set{Dir: dir}, repoDir(t)
}

// inFlight is a driver run happening in the background, so the test can
// signal this process while it is going on — which is the only way to
// exercise the thing under test, since what archimedes forwards is a signal
// aimed at archimedes.
type inFlight struct {
	err      chan error
	progress *syncBuffer
	repo     string
}

// start runs the stub driver and waits until it says it is under way.
// Waiting on that is what makes signalling this process safe: the driver
// writes STARTED only after it is running, which is after the relay that
// catches the signal is armed. Signal before then and the test binary would
// take the default action and die.
func start(t *testing.T, drivers driver.Set, repo string) *inFlight {
	t.Helper()
	f := &inFlight{err: make(chan error, 1), progress: &syncBuffer{}, repo: repo}
	out := filepath.Join(t.TempDir(), "map.md")
	go func() { f.err <- drivers.Run("stub-interruptible", repo, out, f.progress) }()

	waitFor(t, "the driver to start", func() bool { return exists(filepath.Join(repo, "STARTED")) })
	if !exists(filepath.Join(repo, "SCAFFOLD", "thing.txt")) {
		t.Fatal("the driver started without putting anything in the repo, so there is nothing for a rollback to undo")
	}
	return f
}

// interrupt sends sig to this process — archimedes, in the shape these
// tests can reach it — rather than to the driver. That is the whole point:
// a signal the driver already has its own copy of proves nothing.
func (f *inFlight) interrupt(t *testing.T, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(syscall.Getpid(), sig); err != nil {
		t.Fatalf("could not signal this process: %v", err)
	}
}

// finished waits for the run to return and reports what it returned with.
func (f *inFlight) finished(t *testing.T) error {
	t.Helper()
	select {
	case err := <-f.err:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the run never returned after being interrupted")
		return nil
	}
}

// signalsSeen is what the driver recorded of the signals that reached it.
func (f *inFlight) signalsSeen(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.repo, "SIGNALS"))
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// waitFor polls until want is true, failing the test rather than hanging
// forever if it never is. Long enough that reaching the bound is a broken
// test rather than a slow machine.
func waitFor(t *testing.T, what string, want func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if want() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// syncBuffer is progress a test can read while the run is still writing to
// it. The driver's own output and the relay's lines about the signal are
// serialized against each other inside the package; this is the other side
// of that, the reader.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// The whole of what this issue is about, asked of both signals an operator
// or a supervisor actually sends: the driver is told, archimedes is still
// there when it answers, the repo comes back, and the status says which.
func TestASignalToArchimedesReachesTheDriverAndIsWaitedFor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sig    syscall.Signal
		status int
	}{
		{"SIGINT", syscall.SIGINT, 130},
		{"SIGTERM", syscall.SIGTERM, 143},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drivers, repo := interruptible(t)
			run := start(t, drivers, repo)

			run.interrupt(t, tc.sig)
			err := run.finished(t)

			if got := run.signalsSeen(t); len(got) != 1 {
				t.Fatalf("the driver recorded signals %v, want exactly one — a signal aimed at archimedes has to reach it", got)
			}
			// Asked of the moment the run returned, not of some later
			// one: forwarding and then exiting immediately would leave
			// the scaffolding sitting here with the driver still
			// working on it, which is the behaviour being fixed.
			if exists(filepath.Join(repo, "SCAFFOLD")) {
				t.Error("the run returned before the driver had finished rolling the repo back")
			}
			if !strings.Contains(run.progress.String(), "rolled "+repo+" back") {
				t.Errorf("progress = %q, want the driver's own account of the rollback relayed to the operator", run.progress.String())
			}

			if err == nil {
				t.Fatal("an interrupted run reported no error")
			}
			status, stopped := driver.ExitStatus(err)
			if !stopped {
				t.Fatalf("error %q does not read as a run a signal stopped", err)
			}
			if status != tc.status {
				t.Errorf("exit status = %d, want %d — the status the driver chose, passed through", status, tc.status)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("error %q does not name the signal that stopped the run", err)
			}
		})
	}
}

// The operator who hits Ctrl-C twice because the first appeared to do
// nothing. The second copy is not passed on: the rollback it would land in
// is the one thing a driver cannot be interrupted out of halfway, and being
// told what is being waited for is the better answer.
func TestASecondSignalIsNotPassedOnToARollbackInProgress(t *testing.T) {
	drivers, repo := interruptible(t)
	run := start(t, drivers, repo)

	run.interrupt(t, syscall.SIGTERM)
	waitFor(t, "the driver to start rolling back", func() bool { return len(run.signalsSeen(t)) == 1 })

	run.interrupt(t, syscall.SIGTERM)
	waitFor(t, "archimedes to answer the second signal", func() bool {
		return strings.Contains(run.progress.String(), "again")
	})

	err := run.finished(t)

	if got := run.signalsSeen(t); len(got) != 1 {
		t.Errorf("the driver recorded signals %v, want only the first — the second would stop the rollback partway through", got)
	}
	if exists(filepath.Join(repo, "SCAFFOLD")) {
		t.Error("the rollback did not finish, so the repo was left between the two states")
	}
	if status, stopped := driver.ExitStatus(err); !stopped || status != 143 {
		t.Errorf("exit status = %d (stopped=%v), want 143 — a second signal changes nothing about how the run ended", status, stopped)
	}
}
