package reposync_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/testrepo"
)

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

// gitOK runs git in dir, failing the test on error.
func gitOK(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
	}
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
	return strings.TrimSpace(string(out))
}

// makeTargetRepo builds a throwaway bare "origin" plus a clone of it with
// one commit on main, so the syncs have real remote state to fetch from and
// push to without touching the network.
func makeTargetRepo(t *testing.T, tmp, name string) string {
	t.Helper()
	clone := testrepo.New(t, testrepo.Spec{Dir: tmp, Name: name}).Clone
	// The syncs push the branch they're on without naming a refspec.
	gitOK(t, clone, "config", "push.default", "current")
	return clone
}

// instance is an Archimedes instance root wired to target repos, in the
// "../<name>" sibling layout bootstrap produces.
type instance struct {
	root string
	tmp  string
}

func newInstance(t *testing.T, repos ...string) instance {
	t.Helper()
	tmp := t.TempDir()
	inst := instance{root: filepath.Join(tmp, "instance"), tmp: tmp}
	mustMkdirAll(t, inst.root)

	var yaml strings.Builder
	yaml.WriteString("repos:\n")
	for _, name := range repos {
		makeTargetRepo(t, tmp, name+"-repo")
		yaml.WriteString("  - name: " + name + "\n    path: ../" + name + "-repo\n    base_branch: main\n")
	}
	mustWriteFile(t, filepath.Join(inst.root, "repos.yaml"), yaml.String())

	return inst
}

// repoPath is the local checkout for one of the instance's repos.
func (i instance) repoPath(name string) string {
	return filepath.Join(i.tmp, name+"-repo")
}

// setGitHubRemote points a repo's origin at a github.com URL, so
// gitutil.GHSlug derives the same "owner/name" slug it would against a real
// remote. Only safe for syncs that read the remote without pushing to it.
func (i instance) setGitHubRemote(t *testing.T, name, slug string) {
	t.Helper()
	repo := i.repoPath(name)
	gitOK(t, repo, "remote", "set-url", "origin", "https://github.com/"+slug+".git")
}

// writeDossier writes repos/<name>.md with the given House rules body.
func (i instance) writeDossier(t *testing.T, name, houseRules string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(i.root, "repos"))
	mustWriteFile(t, filepath.Join(i.root, "repos", name+".md"),
		"# "+name+"\n\n## House rules\n\n"+houseRules+"\n\n## Known gotchas\nn/a\n")
}

// call is one external command a fakeExec was asked to run.
type call struct {
	name string
	args []string
}

func (c call) joined() string { return c.name + " " + strings.Join(c.args, " ") }

// fakeExec stands in for the ExecFunc seam: it records what it was asked to
// run, answers `gh auth token`, and can be told to fail a command the way
// gh does when a PR already exists.
type fakeExec struct {
	calls    []call
	token    string
	failOn   string
	failWith error
}

func (f *fakeExec) run(name string, args []string, stdout, stderr io.Writer) error {
	f.calls = append(f.calls, call{name: name, args: args})

	if name == "gh" && len(args) >= 2 && args[0] == "auth" && args[1] == "token" {
		token := f.token
		if token == "" {
			token = "fake-token"
		}
		io.WriteString(stdout, token+"\n")
		return nil
	}

	if f.failOn != "" && strings.Contains(strings.Join(append([]string{name}, args...), " "), f.failOn) {
		return f.failWith
	}
	return nil
}

// find returns the first recorded call to name, and whether there was one.
func (f *fakeExec) find(name string) (call, bool) {
	for _, c := range f.calls {
		if c.name == name {
			return c, true
		}
	}
	return call{}, false
}

// count returns how many recorded calls were to name.
func (f *fakeExec) count(name string) int {
	n := 0
	for _, c := range f.calls {
		if c.name == name {
			n++
		}
	}
	return n
}
