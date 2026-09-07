package driver_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/internal/driver"
)

// writeDriver installs a stub driver named name under driversDir: its
// manifest, plus an executable run.sh with the given body (empty body means
// no command file at all).
func writeDriver(t *testing.T, driversDir, name, manifest, body string) {
	t.Helper()
	dir := filepath.Join(driversDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "driver.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if body == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// stubs installs the cast of well-behaved and misbehaving drivers the
// contract tests below exercise, so each case names the driver it needs
// rather than building one. They are an instance's own drivers, with no
// built-in layer beneath: what these tests are about is the contract a
// driver honors, not which layer supplied it (see set_test.go for that).
func stubs(t *testing.T) driver.Set {
	t.Helper()
	dir := t.TempDir()

	writeDriver(t, dir, "stub-ok", "name: stub-ok\noutput_mode: path-parameterized\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nset -euo pipefail\n[ $# -eq 2 ] || { echo 'usage: run.sh <repo-path> <output-path>' >&2; exit 1; }\necho \"stub-ok saw repo $1\" > \"$2\"\n")
	writeDriver(t, dir, "stub-bad-mode", "name: stub-bad-mode\noutput_mode: made-up-mode\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nexit 0\n")
	writeDriver(t, dir, "stub-liar", "name: stub-liar\noutput_mode: path-parameterized\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nexit 0\n")
	writeDriver(t, dir, "stub-not-executable", "name: stub-not-executable\noutput_mode: path-parameterized\ncommand: run.sh\n", "")
	writeDriver(t, dir, "stub-fixed-ok", "name: stub-fixed-ok\noutput_mode: fixed-location\nfixed_path: OUT.md\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nset -euo pipefail\n[ $# -eq 1 ] || { echo 'usage: run.sh <repo-path>' >&2; exit 1; }\necho \"stub-fixed-ok saw repo $1\" > \"$1/OUT.md\"\n")
	writeDriver(t, dir, "stub-fixed-nested", "name: stub-fixed-nested\noutput_mode: fixed-location\nfixed_path: .stub/memory/OUT.md\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nset -euo pipefail\n[ $# -eq 1 ] || { echo 'usage: run.sh <repo-path>' >&2; exit 1; }\nmkdir -p \"$1/.stub/memory\"\necho \"stub-fixed-nested saw repo $1\" > \"$1/.stub/memory/OUT.md\"\n")
	writeDriver(t, dir, "stub-fixed-liar", "name: stub-fixed-liar\noutput_mode: fixed-location\nfixed_path: OUT.md\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nexit 0\n")
	writeDriver(t, dir, "stub-fixed-no-path", "name: stub-fixed-no-path\noutput_mode: fixed-location\ncommand: run.sh\n",
		"#!/usr/bin/env bash\necho 'should not have been invoked' >&2\ntouch \"$1/INVOKED\"\nexit 1\n")
	writeDriver(t, dir, "stub-fails", "name: stub-fails\noutput_mode: path-parameterized\ncommand: run.sh\n",
		"#!/usr/bin/env bash\necho 'driver blew up' >&2\nexit 3\n")

	return driver.Set{Dir: dir}
}

func repoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertMissing(t *testing.T, path, what string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s: %s exists but should not", what, path)
	}
}

func TestUnknownDriverFailsAndWritesNothing(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "unknown.md")

	err := drivers.Run("nonexistent-driver", repo, out, io.Discard)
	if err == nil {
		t.Fatal("expected an error for an unknown driver, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent-driver") {
		t.Errorf("error %q does not name the unknown driver", err)
	}
	assertMissing(t, out, "unknown driver")
}

func TestUnsupportedOutputModeFailsAndWritesNothing(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "bad-mode.md")

	err := drivers.Run("stub-bad-mode", repo, out, io.Discard)
	if err == nil {
		t.Fatal("expected an error for an unsupported output_mode, got nil")
	}
	if !strings.Contains(err.Error(), "made-up-mode") {
		t.Errorf("error %q does not name the declared mode", err)
	}
	assertMissing(t, out, "unsupported output_mode")
}

func TestNonExecutableCommandFails(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "not-executable.md")

	err := drivers.Run("stub-not-executable", repo, out, io.Discard)
	if err == nil {
		t.Fatal("expected an error for a missing/non-executable command, got nil")
	}
	if !strings.Contains(err.Error(), "run.sh") {
		t.Errorf("error %q does not name the command", err)
	}
}

func TestPathParameterizedWritesToExactPath(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "nested", "dir", "ok.md")

	if err := drivers.Run("stub-ok", repo, out, io.Discard); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("driver did not write the output path (parent dirs created?): %v", err)
	}
	if want := "stub-ok saw repo " + repo; !strings.Contains(string(got), want) {
		t.Errorf("output = %q, want it to contain %q", got, want)
	}
}

func TestPathParameterizedDriverIsGivenAnAbsoluteRepoPath(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "abs.md")

	rel, err := filepath.Rel(mustGetwd(t), repo)
	if err != nil {
		t.Skipf("no relative path from cwd to %s: %v", repo, err)
	}

	if err := drivers.Run("stub-ok", rel, out, io.Discard); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if want := "stub-ok saw repo " + repo; !strings.Contains(string(got), want) {
		t.Errorf("output = %q, want the driver to have been handed the absolute path %q", got, repo)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

func TestZeroExitWithoutOutputIsAnError(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "liar.md")

	err := drivers.Run("stub-liar", repo, out, io.Discard)
	if err == nil {
		t.Fatal("expected an error when the driver exits 0 without writing, got nil")
	}
	if !strings.Contains(err.Error(), out) {
		t.Errorf("error %q does not name the missing output path %q", err, out)
	}
}

func TestNonZeroExitIsAnError(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "fails.md")

	err := drivers.Run("stub-fails", repo, out, io.Discard)
	if err == nil {
		t.Fatal("expected an error when the driver exits non-zero, got nil")
	}
	assertMissing(t, out, "failing driver")
}

func TestFixedLocationHarvestsToRequestedPathLeavingNoTrace(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "harvested", "fixed.md")

	if err := drivers.Run("stub-fixed-ok", repo, out, io.Discard); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("fixed-location output was not harvested to the requested path: %v", err)
	}
	if want := "stub-fixed-ok saw repo " + repo; !strings.Contains(string(got), want) {
		t.Errorf("harvested output = %q, want it to contain %q", got, want)
	}
	assertMissing(t, filepath.Join(repo, "OUT.md"), "harvest moves rather than copies, so the target repo")
}

// A fixed_path can be nested, because a driver wrapping a tool that
// scaffolds itself into the repo has no say in where that tool writes (the
// spec-kit driver's is .specify/memory/constitution.md). Moving the file
// out then leaves its directories behind, holding nothing — which no
// porcelain check would catch, since git doesn't track directories, but
// which is a trace of the run all the same.
func TestFixedLocationPrunesDirectoriesTheHarvestEmpties(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "nested.md")

	if err := drivers.Run("stub-fixed-nested", repo, out, io.Discard); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("nested fixed_path was not harvested to the requested path: %v", err)
	}
	if want := "stub-fixed-nested saw repo " + repo; !strings.Contains(string(got), want) {
		t.Errorf("harvested output = %q, want it to contain %q", got, want)
	}
	assertMissing(t, filepath.Join(repo, ".stub", "memory", "OUT.md"), "the harvested file")
	assertMissing(t, filepath.Join(repo, ".stub"), "the directory tree the harvest emptied")

	// Pruning walks up from the fixed_path, so it has to stop at the first
	// directory still holding something rather than eating the repo.
	if _, err := os.Stat(filepath.Join(repo, "README.md")); err != nil {
		t.Errorf("pruning removed content that was not the harvest's to remove: %v", err)
	}
}

// A fixed_path sitting in a directory that was already there is the case
// pruning must not touch: the directory is not the run's to remove.
func TestFixedLocationLeavesDirectoriesItDidNotEmpty(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	kept := filepath.Join(repo, ".stub", "memory", "keep-me.txt")
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "nested-shared.md")

	if err := drivers.Run("stub-fixed-nested", repo, out, io.Discard); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("pruning removed a directory that still held something: %v", err)
	}
}

func TestFixedLocationWithoutFixedPathFailsBeforeInvokingTheDriver(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "no-fixed-path.md")

	err := drivers.Run("stub-fixed-no-path", repo, out, io.Discard)
	if err == nil {
		t.Fatal("expected an error for fixed-location with no fixed_path, got nil")
	}
	if !strings.Contains(err.Error(), "fixed_path") {
		t.Errorf("error %q does not name the missing field", err)
	}
	assertMissing(t, filepath.Join(repo, "INVOKED"), "the driver")
	assertMissing(t, out, "missing fixed_path")
}

func TestFixedLocationZeroExitWithoutWritingIsAnError(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "fixed-liar.md")

	err := drivers.Run("stub-fixed-liar", repo, out, io.Discard)
	if err == nil {
		t.Fatal("expected an error when a fixed-location driver exits 0 without writing, got nil")
	}
	if want := filepath.Join(repo, "OUT.md"); !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name the missing fixed_path location %q", err, want)
	}
	assertMissing(t, out, "fixed-location liar")
}

func TestDriverOutputReachesProgressWriter(t *testing.T) {
	drivers, repo := stubs(t), repoDir(t)
	out := filepath.Join(t.TempDir(), "fails.md")

	var progress strings.Builder
	_ = drivers.Run("stub-fails", repo, out, &progress)

	if !strings.Contains(progress.String(), "driver blew up") {
		t.Errorf("progress = %q, want the driver's own output", progress.String())
	}
}
