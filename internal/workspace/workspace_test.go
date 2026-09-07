package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/internal/workspace"
)

// stubHerdr puts a fake `herdr` on PATH that appends its argv to a log
// file, writes stderr, and exits with code. Tests can then assert on the
// exact invocation without a real herdr server anywhere near it.
func stubHerdr(t *testing.T, code int, stderr string) (argvLog string) {
	t.Helper()
	dir := t.TempDir()
	argvLog = filepath.Join(dir, "argv")

	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" >> " + argvLog + "\n" +
		"printf '%s' " + shellQuote(stderr) + " >&2\n" +
		"exit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	return argvLog
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// recordedArgv reads back what the stub was called with.
func recordedArgv(t *testing.T, argvLog string) string {
	t.Helper()
	data, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("stub herdr was never invoked: %v", err)
	}
	return strings.Join(strings.Fields(string(data)), " ")
}

func TestSelect(t *testing.T) {
	// Opting out is the default: no configuration means spawn behaves
	// exactly as it did before this integration existed.
	for _, off := range []string{"", "off", "none", "  ", "OFF"} {
		got, err := workspace.Select(off)
		if err != nil {
			t.Errorf("Select(%q): unexpected error: %v", off, err)
		}
		if got != nil {
			t.Errorf("Select(%q) = %+v, want nil (disabled)", off, got)
		}
	}

	for _, on := range []string{"herdr", "HERDR", " herdr "} {
		got, err := workspace.Select(on)
		if err != nil {
			t.Fatalf("Select(%q): %v", on, err)
		}
		if got == nil || got.Name != "herdr" {
			t.Errorf("Select(%q) = %+v, want the herdr integration", on, got)
		}
	}

	// A typo must not silently disable the pane the operator asked for.
	if _, err := workspace.Select("hrdr"); err == nil {
		t.Error("expected an error for an unknown integration, got nil")
	}
}

func TestHerdrOpensTheExistingWorktreeInTheBackground(t *testing.T) {
	argvLog := stubHerdr(t, 0, "")

	integration, err := workspace.Select("herdr")
	if err != nil {
		t.Fatal(err)
	}
	err = integration.Open(workspace.Request{
		RepoPath: "/instance/target-repo",
		Path:     "/instance/target-repo-worktrees/widget-fix",
		Label:    "target:widget-fix",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// `open`, not `create`: git already made the checkout, so herdr adopts
	// it rather than making a second one somewhere else. --cwd names the
	// repo the worktree belongs to; without it herdr resolves worktrees
	// against its own process directory and never finds this path.
	want := "worktree open --cwd /instance/target-repo " +
		"--path /instance/target-repo-worktrees/widget-fix --label target:widget-fix --no-focus"
	if got := recordedArgv(t, argvLog); got != want {
		t.Errorf("herdr invocation\n got: %s\nwant: %s", got, want)
	}
}

func TestHerdrFocusesWhenAsked(t *testing.T) {
	argvLog := stubHerdr(t, 0, "")

	integration, _ := workspace.Select("herdr")
	if err := integration.Open(workspace.Request{RepoPath: "/repo", Path: "/wt", Label: "l", Focus: true}); err != nil {
		t.Fatalf("Open: %v", err)
	}

	argv := recordedArgv(t, argvLog)
	if !strings.Contains(argv, "--focus") || strings.Contains(argv, "--no-focus") {
		t.Errorf("expected a focusing invocation, got: %s", argv)
	}
}

// The integration tool being absent is the normal case on any machine that
// doesn't use it, so it has to be a recognizable, non-fatal condition
// rather than an opaque exec failure.
func TestHerdrMissingBinaryIsUnavailableNotFatal(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	integration, _ := workspace.Select("herdr")
	err := integration.Open(workspace.Request{RepoPath: "/repo", Path: "/wt", Label: "l"})
	if !errors.Is(err, workspace.ErrUnavailable) {
		t.Errorf("got %v, want an error matching ErrUnavailable", err)
	}
}

// herdr reports server-side problems as JSON on stderr with exit status 1;
// swallowing that leaves the operator with "it didn't work" and nothing else.
func TestHerdrFailureReportsWhatHerdrSaid(t *testing.T) {
	stubHerdr(t, 1, `{"error":"server_not_running"}`)

	integration, _ := workspace.Select("herdr")
	err := integration.Open(workspace.Request{RepoPath: "/repo", Path: "/wt", Label: "l"})
	if err == nil {
		t.Fatal("expected an error when herdr exits non-zero, got nil")
	}
	if !strings.Contains(err.Error(), "server_not_running") {
		t.Errorf("error dropped herdr's own diagnostic: %v", err)
	}
}
