package spawn_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/testrepo"
)

// gitOK runs git in dir, failing the test on error.
func gitOK(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
	}
}

// gitCommit commits everything staged in dir with a fixed identity, so
// tests don't depend on the machine's git config.
func gitCommit(t *testing.T, dir, message string, extraArgs ...string) {
	t.Helper()
	args := append([]string{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", message}, extraArgs...)
	gitOK(t, dir, args...)
}

// gitOut runs git in dir and returns trimmed stdout, failing the test on error.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v (in %s): %v", args, dir, err)
	}
	return string(bytes.TrimSpace(out))
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// makeTargetRepo builds the shape spawn needs to be exercised for real: a
// bare "origin" plus a clone with a normal .gitignore and one commit on
// main, so spawn's fetch-first behavior has real remote state to pull, and
// so the repo's own tracked .gitignore is there to be left alone.
func makeTargetRepo(t *testing.T, tmp, name string) (clonePath string) {
	t.Helper()
	return testrepo.New(t, testrepo.Spec{
		Dir:   tmp,
		Name:  name,
		Files: map[string]string{".gitignore": "*.log\n"},
	}).Clone
}

// instance is an Archimedes instance root wired to two target repos, the
// multi-repo shape the worktree pattern exists for.
type instance struct {
	root        string
	targetRepo  string
	targetRepo2 string
}

// newInstance builds an instance whose repos.yaml points at two sibling
// target repos, the "../<name>" layout bootstrap produces.
func newInstance(t *testing.T) instance {
	t.Helper()
	tmp := t.TempDir()

	inst := instance{
		root:        filepath.Join(tmp, "instance"),
		targetRepo:  makeTargetRepo(t, tmp, "target-repo"),
		targetRepo2: makeTargetRepo(t, tmp, "target-repo2"),
	}

	mustMkdirAll(t, inst.root)
	mustWriteFile(t, filepath.Join(inst.root, "repos.yaml"),
		"repos:\n"+
			"  - name: target\n    path: ../target-repo\n    base_branch: main\n"+
			"  - name: target2\n    path: ../target-repo2\n    base_branch: main\n")

	return inst
}

// workSlug creates work/<slug>/ in the instance and returns its path.
func (i instance) workSlug(t *testing.T, slug string) string {
	t.Helper()
	dir := filepath.Join(i.root, "work", slug)
	mustMkdirAll(t, dir)
	return dir
}
