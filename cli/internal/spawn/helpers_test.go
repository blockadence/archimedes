package spawn_test

import (
	"os"
	"path/filepath"
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

// makeTargetRepo builds the shape spawn needs to be exercised for real: a
// bare "origin" plus a clone with a normal .gitignore and one commit on
// main, so spawn's fetch-first behavior has real remote state to pull, and
// so the repo's own tracked .gitignore is there to be left alone.
func makeTargetRepo(t *testing.T, tmp, name string) testrepo.Repo {
	t.Helper()
	return testrepo.New(t, testrepo.Spec{
		Dir:   tmp,
		Name:  name,
		Files: map[string]string{".gitignore": "*.log\n"},
	})
}

// instance is an Archimedes instance root wired to two target repos, the
// multi-repo shape the worktree pattern exists for.
type instance struct {
	root    string
	target  testrepo.Repo
	target2 testrepo.Repo
}

// newInstance builds an instance whose repos.yaml points at two sibling
// target repos, the "../<name>" layout bootstrap produces.
func newInstance(t *testing.T) instance {
	t.Helper()
	tmp := t.TempDir()

	inst := instance{
		root:    filepath.Join(tmp, "instance"),
		target:  makeTargetRepo(t, tmp, "target-repo"),
		target2: makeTargetRepo(t, tmp, "target-repo2"),
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
