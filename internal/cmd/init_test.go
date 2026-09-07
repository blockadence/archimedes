package cmd

import (
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
