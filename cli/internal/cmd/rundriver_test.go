package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubDriverInstance scaffolds an instance root whose drivers/ holds one
// path-parameterized driver that writes body to wherever it is told.
func stubDriverInstance(t *testing.T, root, name, body string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "drivers", name, "driver.yaml"),
		"name: "+name+"\noutput_mode: path-parameterized\ncommand: run.sh\n")
	writeExecutable(t, filepath.Join(root, "drivers", name, "run.sh"),
		"#!/usr/bin/env bash\nset -euo pipefail\necho 'driver chatter'\nprintf '%s' '"+body+"' > \"$2\"\n")
}

func TestRunDriverWritesToTheExactRequestedPath(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "instance")
	stubDriverInstance(t, root, "stub", "mapped")
	repo := filepath.Join(tmp, "app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "maps", "app", "CONTEXT.md")

	stdout := execute(t, "run-driver", "--root", root, "stub", repo, out)

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected the map at %s: %v", out, err)
	}
	if string(got) != "mapped" {
		t.Errorf("context map = %q, want the driver's output", got)
	}
	if !strings.Contains(stdout, out) {
		t.Errorf("stdout %q should report where the map landed", stdout)
	}
}

// The driver's own output is progress, not the result, so it belongs on
// stderr — the same split every other subcommand makes, and what lets the
// result stream be piped somewhere.
func TestRunDriverKeepsDriverChatterOffTheResultStream(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "instance")
	stubDriverInstance(t, root, "stub", "mapped")

	stdout, stderr, err := executeStreams(t, "run-driver", "--root", root, "stub", tmp, filepath.Join(tmp, "CONTEXT.md"))
	if err != nil {
		t.Fatalf("run-driver: %v", err)
	}
	if strings.Contains(stdout, "driver chatter") {
		t.Errorf("driver output leaked onto stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "driver chatter") {
		t.Errorf("driver output should reach stderr, got %q", stderr)
	}
}

// Drivers are looked up the same way a mapping pass looks them up, so
// exercising one directly and having it run in a pass can't disagree about
// which drivers/ they mean.
func TestRunDriverHonorsTheDriversDirOverride(t *testing.T) {
	tmp := t.TempDir()
	elsewhere := filepath.Join(tmp, "shared")
	stubDriverInstance(t, elsewhere, "stub", "from elsewhere")
	// Not where --root would look: an instance with no drivers/ at all.
	root := filepath.Join(tmp, "instance")
	writeFile(t, filepath.Join(root, "repos.yaml"), "repos: []\n")

	t.Setenv(driversDirEnvVar, filepath.Join(elsewhere, "drivers"))
	out := filepath.Join(tmp, "CONTEXT.md")
	execute(t, "run-driver", "--root", root, "stub", tmp, out)

	got, err := os.ReadFile(out)
	if err != nil || string(got) != "from elsewhere" {
		t.Errorf("context map = %q (err %v), want the override's driver to have run", got, err)
	}
}

// Naming a driver that isn't there is a misconfiguration, not something to
// paper over — and nothing is written for it.
func TestRunDriverUnknownDriverFailsAndWritesNothing(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "instance")
	stubDriverInstance(t, root, "stub", "mapped")
	out := filepath.Join(tmp, "CONTEXT.md")

	err := executeErr(t, "run-driver", "--root", root, "nope", tmp, out)
	if !strings.Contains(err.Error(), "unknown driver: nope") {
		t.Errorf("error = %v, want it to name the unknown driver", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Errorf("%s was written for a driver that doesn't exist", out)
	}
}

func TestRunDriverRequiresThreeArguments(t *testing.T) {
	executeErr(t, "run-driver", "stub", "/tmp")
}
