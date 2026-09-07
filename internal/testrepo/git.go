package testrepo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Git runs one git command in dir and fails the test if it doesn't succeed,
// reporting git's own output so the failure says why. Use it for the steps a
// fixture takes for effect — init, commit, push, checkout.
//
// An empty dir runs the command wherever the test binary is, which is what
// the directory-taking forms (clone, init <path>) want.
func Git(t testing.TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v (in %q): %v\n%s", args, dir, err, out)
	}
}

// GitOut runs one git command in dir and returns its stdout with surrounding
// whitespace trimmed, failing the test with git's stderr if it doesn't
// succeed. Trimming is the helper's job because git terminates nearly
// everything it prints with a newline that no caller wants: a call site
// comparing against "main" or "" should not have to say so.
func GitOut(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v (in %q): %v\n%s", args, dir, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// IsolateGit points git at a config of this test's own, with an identity in
// it, for tests that commit into a repository they created rather than one
// New built. Without it such a test passes or fails on whether the machine
// running the suite happens to have a global user.name — and on a machine
// that has one, silently records it.
func IsolateGit(t testing.TB) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = t\n\temail = t@t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}
