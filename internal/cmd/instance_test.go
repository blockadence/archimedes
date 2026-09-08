package cmd

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/bootstrap"
	"github.com/blockadence/gh-archimedes/internal/testrepo"
)

// An instance is not any one command's output. `init` writes the template
// tree; `bootstrap` writes a manifest entry, a dossier stub and a
// regenerated map into what init left, and clones the org's repos as
// siblings of it; `spawn` writes a unit of work's bookkeeping into work/.
// Anything asserted about an instance as a whole -- what it contains, what
// its files may say -- needs the artifact all of them produced, so it is
// scaffolded once here and the tests below and in root_test.go read it.
//
// A subcommand added later that writes into an instance belongs in
// scaffoldInstance, and is then held to everything asserted about one
// without any of those tests naming it.

const (
	// orgRepoName is the one repo the fake org owns. Its name reaches the
	// instance three times over (repos.yaml, repos/<name>.md,
	// WORKSPACE-MAP.md) and names the sibling checkout beside it.
	orgRepoName = "service-a"
	// workSlug is the one unit of work spawned in it.
	workSlug = "widget-fix"
)

// scaffoldInstance builds the instance an operator ends up with after the
// commands that create one and start work in it, and returns its root along
// with the parent directory holding it.
//
// The parent matters as much as the root: bootstrap clones the org's repos
// as *siblings* of the instance, and spawn puts their worktrees beside them
// again. So what comes back is a directory holding an instance and other
// people's repositories next to it, which is the arrangement on a real
// machine and the one anything walking an instance has to respect.
func scaffoldInstance(t *testing.T) (root, parent string) {
	t.Helper()
	parent = initParent(t)

	execute(t, "init", "widgets", parent)
	root = filepath.Join(parent, "widgets")

	// Beside the instance, where bootstrap will look for a checkout and,
	// finding none, clone one. The seed clone is kept out of the path
	// bootstrap clones into.
	origin := testrepo.New(t, testrepo.Spec{
		Dir:    parent,
		Name:   orgRepoName,
		Origin: orgRepoName + "-origin.git",
		Clone:  orgRepoName + "-seed",
	}).Origin

	// bootstrap goes to its own package rather than through the command,
	// which hardcodes the discovery that talks to GitHub. RepoLister is the
	// seam that package exports for exactly this. Every other step here is
	// the real command.
	var out bytes.Buffer
	err := bootstrap.Run(bootstrap.Options{
		Root: root,
		Org:  "acme",
		List: func(string) ([]bootstrap.OrgRepo, error) {
			return []bootstrap.OrgRepo{{Name: orgRepoName, SSHURL: origin}}, nil
		},
	}, &out, io.Discard)
	if err != nil {
		t.Fatalf("bootstrap: %v\n%s", err, out.String())
	}

	// `--workspace off` rather than the default, which reads the
	// environment: a developer running the suite with a workspace manager
	// configured instance-wide would otherwise have this open panes.
	execute(t, "spawn", workSlug, orgRepoName, "--root", root, "--workspace", "off")

	return root, parent
}

// instanceFiles reads every file in the instance at root, keyed by its path
// relative to root.
//
// The instance's own git history is excluded, for two reasons that both
// apply: two scaffolding runs commit at two times and so differ by hash for
// a reason that is never the one under test, and its contents are
// compressed objects rather than the prose anything here reads.
func instanceFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = string(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// What init, bootstrap and spawn write is the instance's own content from
// the moment it lands: committed to its history, edited by its operator,
// read by teammates and agents on machines that may have the other install
// or neither. So none of it may record which install scaffolded it -- one
// identical instance, whoever ran the commands and however they had the
// tool.
//
// One test over the whole instance rather than one per command: the rule is
// about the artifact, and a command added later is held to it by being part
// of what scaffoldInstance produces.
//
// The two scaffolds are compared byte for byte, with nothing normalized
// away. That is only possible because an instance records no path that is
// true on one machine: two runs land in two different temp directories and
// write the same files anyway. `spawn`'s worktree column was the exception
// and is not any more (issue 46, and internal/worktree), so a normalizing
// step re-introduced here would be hiding a regression of it.
func TestAScaffoldedInstanceIsTheSameHoweverItWasInvoked(t *testing.T) {
	scaffold := func(ghExtension string) map[string]string {
		t.Setenv("GH_EXTENSION", ghExtension)
		root, _ := scaffoldInstance(t)
		return instanceFiles(t, root)
	}

	standalone, extension := scaffold(""), scaffold("1")

	if len(standalone) == 0 {
		t.Fatal("scaffolded nothing to compare")
	}
	for path, want := range standalone {
		got, ok := extension[path]
		if !ok {
			t.Errorf("the gh extension install scaffolds no %s", path)
			continue
		}
		if got != want {
			t.Errorf("%s differs by install:\nstandalone:\n%s\ngh extension:\n%s", path, want, got)
		}
	}
	for path := range extension {
		if _, ok := standalone[path]; !ok {
			t.Errorf("the gh extension install scaffolds an extra %s", path)
		}
	}
}
