package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/testrepo"
)

// init is the one subcommand that runs before an instance exists, so there
// is no fixture instance to build here — only a parent directory to put one
// in, and a git config of the test's own so the scaffolding commit doesn't
// depend on the machine's identity.
func initParent(t *testing.T) string {
	t.Helper()
	testrepo.IsolateGit(t)
	return t.TempDir()
}

func TestInitScaffoldsAnInstanceFromTheEmbeddedTemplate(t *testing.T) {
	parent := initParent(t)

	out := execute(t, "init", "widgets", parent)

	dest := filepath.Join(parent, "widgets")
	if _, err := os.Stat(filepath.Join(dest, "repos.yaml")); err != nil {
		t.Errorf("no instance at %s: %v", dest, err)
	}
	if !strings.Contains(out, dest) {
		t.Errorf("output does not say where the instance is:\n%s", out)
	}
	// Whoever just ran this has never used the tool before; the one thing
	// they need next is the command that fills the instance in.
	if !strings.Contains(out, "bootstrap") {
		t.Errorf("output does not point at the next step:\n%s", out)
	}
}

func TestInitRefusesToScaffoldOverAnExistingDirectory(t *testing.T) {
	parent := initParent(t)
	dest := filepath.Join(parent, "widgets")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	err := executeErr(t, "init", "widgets", parent)

	if !strings.Contains(err.Error(), dest) {
		t.Errorf("error does not name the path in the way: %v", err)
	}
}

func TestInitNeedsBothANameAndAParentDirectory(t *testing.T) {
	parent := initParent(t)

	executeErr(t, "init", "widgets")
	executeErr(t, "init", "widgets", parent, "extra")
}

// init's parting line is the one piece of runtime output that tells an
// operator what to type next, and it is the first thing anybody sees. Under
// a gh extension install `archimedes bootstrap` is not a command they have.
func TestInitPointsAtTheNextCommandInTheFormTheOperatorCanRun(t *testing.T) {
	for _, tc := range []struct{ ghExtension, want string }{
		{"", "&& archimedes bootstrap"},
		{"1", "&& gh archimedes bootstrap"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("GH_EXTENSION", tc.ghExtension)

			out := execute(t, "init", "widgets", initParent(t))

			if !strings.Contains(out, tc.want) {
				t.Errorf("init's next step does not say %q:\n%s", tc.want, out)
			}
		})
	}
}

// What init writes is the instance's own content from the moment it lands:
// committed to its history, edited by its operator, read by teammates and
// agents on machines that may have the other install or neither. So unlike
// the parting line above, none of it may record which install scaffolded it
// -- one identical instance, whoever ran the command.
func TestInitWritesTheSameInstanceHoweverItWasInvoked(t *testing.T) {
	scaffold := func(ghExtension string) map[string]string {
		t.Setenv("GH_EXTENSION", ghExtension)
		parent := initParent(t)
		execute(t, "init", "widgets", parent)

		root := filepath.Join(parent, "widgets")
		files := map[string]string{}
		// The instance's own git history is excluded: two runs commit at
		// two times, so it differs by hash for a reason that is not this.
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
