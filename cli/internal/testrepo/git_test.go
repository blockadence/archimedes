package testrepo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/testrepo"
)

// fatalPanic is what recorder.Fatalf panics with, so that — like the real
// t.Fatalf — nothing after the failing call runs.
type fatalPanic struct{}

// recorder stands in for *testing.T so a test can assert on how the helpers
// fail without failing itself. testing.TB can't be implemented outside the
// testing package, so it embeds the real one and overrides what it watches.
// That override is only Fatalf: a helper that ever failed some other way
// would reach the embedded real t and kill the test doing the asserting, so
// "the helpers fail via Fatalf" is a contract this fake depends on.
type recorder struct {
	testing.TB
	failed bool
	msg    string
}

func (r *recorder) Helper() {}

func (r *recorder) Fatalf(format string, args ...any) {
	r.failed = true
	r.msg = fmt.Sprintf(format, args...)
	panic(fatalPanic{})
}

// expectFatal runs fn, which is expected to fail its test, and returns the
// message it failed with.
func expectFatal(t *testing.T, fn func(testing.TB)) string {
	t.Helper()
	rec := &recorder{TB: t}
	func() {
		defer func() {
			if p := recover(); p != nil {
				if _, ok := p.(fatalPanic); !ok {
					panic(p)
				}
			}
		}()
		fn(rec)
	}()
	if !rec.failed {
		t.Fatal("expected the helper to fail the test, but it did not")
	}
	return rec.msg
}

func TestGitRunsTheCommandInTheDirectoryItIsGiven(t *testing.T) {
	dir := t.TempDir()

	testrepo.Git(t, dir, "init", "-q")

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("git init left no .git behind: %v", err)
	}
}

func TestGitFailsTheTestWhenGitDoes(t *testing.T) {
	repo := testrepo.New(t, testrepo.Spec{Dir: t.TempDir(), Name: "app"})

	msg := expectFatal(t, func(tb testing.TB) {
		testrepo.Git(tb, repo.Clone, "checkout", "no-such-branch")
	})

	if !strings.Contains(msg, "no-such-branch") {
		t.Errorf("failure message = %q, want the arguments in it", msg)
	}
	if !strings.Contains(msg, repo.Clone) {
		t.Errorf("failure message = %q, want the directory in it", msg)
	}
	// git's own complaint is the whole point of failing loudly.
	if !strings.Contains(msg, "did not match any file") {
		t.Errorf("failure message = %q, want git's own output in it", msg)
	}
}

func TestGitOutReturnsStdoutWithSurroundingWhitespaceTrimmed(t *testing.T) {
	repo := testrepo.New(t, testrepo.Spec{Dir: t.TempDir(), Name: "app"})

	if got := testrepo.GitOut(t, repo.Clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("GitOut = %q, want the branch name with git's trailing newline gone", got)
	}
}

func TestGitOutReturnsEmptyForACommandThatPrintsNothing(t *testing.T) {
	repo := testrepo.New(t, testrepo.Spec{Dir: t.TempDir(), Name: "app"})

	if got := testrepo.GitOut(t, repo.Clone, "status", "--porcelain"); got != "" {
		t.Errorf("GitOut = %q, want empty for a clean tree", got)
	}
}

func TestGitOutFailsTheTestWhenGitDoes(t *testing.T) {
	repo := testrepo.New(t, testrepo.Spec{Dir: t.TempDir(), Name: "app"})

	msg := expectFatal(t, func(tb testing.TB) {
		testrepo.GitOut(tb, repo.Clone, "rev-parse", "no-such-ref")
	})

	if !strings.Contains(msg, "no-such-ref") {
		t.Errorf("failure message = %q, want the arguments in it", msg)
	}
	// stdout alone says nothing about why rev-parse failed; stderr does.
	if !strings.Contains(msg, "unknown revision") {
		t.Errorf("failure message = %q, want git's stderr in it", msg)
	}
}
