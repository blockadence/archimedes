package cmd

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/blockadence/gh-archimedes"
)

// Installed as a gh extension the binary is exactly the same program, run
// by gh with the subcommand and flags forwarded verbatim -- so everything
// below the surface is already identical and there is nothing to assert
// about it. What differs is the one thing gh cannot forward: the operator
// typed `gh archimedes`, and help that answers `archimedes` is telling them
// to run something that isn't installed under that name.
//
// gh announces itself with GH_EXTENSION=1, which is documented in
// `gh help environment` precisely so an extension can tell the two apart.

func TestUsageNamesTheBinaryWhenRunOnItsOwn(t *testing.T) {
	t.Setenv("GH_EXTENSION", "")

	root := newRootCmd()

	if got := root.CommandPath(); got != "archimedes" {
		t.Errorf("root command path = %q, want archimedes", got)
	}
	if got := usageOf(t, root, "spawn"); !strings.Contains(got, "archimedes spawn") {
		t.Errorf("spawn usage = %q, want it to start with `archimedes spawn`", got)
	}
}

func TestUsageNamesGhWhenGhDispatchedUs(t *testing.T) {
	t.Setenv("GH_EXTENSION", "1")

	root := newRootCmd()

	if got := root.CommandPath(); got != "gh archimedes" {
		t.Errorf("root command path = %q, want gh archimedes", got)
	}
	if got := root.UseLine(); !strings.HasPrefix(got, "gh archimedes") {
		t.Errorf("root use line = %q, want it to start with `gh archimedes`", got)
	}
	if got := usageOf(t, root, "spawn"); !strings.Contains(got, "gh archimedes spawn") {
		t.Errorf("spawn usage = %q, want it to name the command the operator typed", got)
	}
}

// Every subcommand, not just the one spot-checked above: a stale name in
// any of them sends the same operator to the same dead end.
func TestEverySubcommandNamesGhWhenGhDispatchedUs(t *testing.T) {
	t.Setenv("GH_EXTENSION", "1")

	for _, sub := range newRootCmd().Commands() {
		if !strings.HasPrefix(sub.UseLine(), "gh archimedes ") {
			t.Errorf("%s use line = %q, want it under `gh archimedes`", sub.Name(), sub.UseLine())
		}
	}
}

func usageOf(t *testing.T, root *cobra.Command, name string) string {
	t.Helper()
	for _, sub := range root.Commands() {
		if sub.Name() == name {
			return sub.UseLine()
		}
	}
	t.Fatalf("no %s subcommand", name)
	return ""
}

// The display-name annotation fixes every usage line at once, but not the
// prose around them, and not the "run this next" line a command prints when
// it succeeds. Those are ordinary strings, and a new one that spells the
// name out sends an operator who installed the extension to a command they
// have not got -- the failure this catches, since nothing else would until
// somebody tried it.
//
// Scoped to `archimedes <subcommand>`, because naming the program as a
// thing is not the same as telling someone to type it: "the drivers
// archimedes ships" is true under either install and stays as it is.
func TestNoHelpTextNamesACommandAnExtensionUserCannotRun(t *testing.T) {
	t.Setenv("GH_EXTENSION", "1")
	root := newRootCmd()
	subcommands := subcommandNames(root)

	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		texts := map[string]string{
			"short":       c.Short,
			"long":        c.Long,
			"use line":    c.UseLine(),
			"example":     c.Example,
			"flag usages": c.Flags().FlagUsages(),
		}

		for where, text := range texts {
			for _, named := range bareCommandsIn(text, subcommands) {
				t.Errorf("%s's %s says %q where an extension install needs `gh archimedes %s`",
					c.Name(), where, "archimedes "+named, named)
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

// subcommandNames is what bareCommandsIn matches against: the names the
// tree actually registers, so the check follows a command being added
// rather than a list somebody has to remember to extend.
func subcommandNames(root *cobra.Command) map[string]bool {
	names := map[string]bool{}
	for _, sub := range root.Commands() {
		names[sub.Name()] = true
	}
	return names
}

// bareCommandsIn finds every "archimedes <subcommand>" in text that isn't
// already reached through gh.
func bareCommandsIn(text string, subcommands map[string]bool) []string {
	var found []string
	for i := 0; ; {
		at := strings.Index(text[i:], "archimedes ")
		if at < 0 {
			return found
		}
		at += i
		i = at + len("archimedes ")

		if strings.HasSuffix(text[:at], "gh ") {
			continue
		}
		word := text[i:]
		if end := strings.IndexAny(word, " \n\t\"`,.;"); end >= 0 {
			word = word[:end]
		}
		if subcommands[word] {
			found = append(found, word)
		}
	}
}

// The instance template is on the far side of the line the test above
// draws. What that one guards is what the tool *prints*, which can be built
// from invocation.Name() because it is composed fresh on the machine
// reading it. The template's files are *written*: init copies them into an
// instance that then commits them, edits them, and shares them with
// teammates and agents who may have either install or none. So they cannot
// name the invoking form -- and they cannot name one fixed form either,
// since half the readers have not got it.
//
// The answer is to name the subcommand alone (`spawn`, the `drivers`
// listing) everywhere, and to state the two forms it is prefixed with once,
// in the README, where an operator first meets the instance. The three
// tests below are the parts of that: nothing sends a reader to a command,
// the one place that explains the convention still does, and the file an
// agent reads still points at it.
//
// They live here rather than beside the template because this is the same
// check as the one above, against the same list of real subcommands.

func TestTheInstanceTemplateNamesNoCommandHalfItsReadersHaventGot(t *testing.T) {
	subcommands := subcommandNames(newRootCmd())

	tmpl := archimedes.Template()
	// Every file, `scaffolding/` included. Those are pushed into other
	// people's repositories and so stay fixed under either install, which is
	// a stricter rule than this one rather than a different one -- there is
	// nothing here for them to fail, and no reason to carve them out.
	err := fs.WalkDir(tmpl, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := fs.ReadFile(tmpl, path)
		if err != nil {
			return err
		}
		// Scanned as one line: the template is hard-wrapped, so the mention
		// this is most likely to meet is a `archimedes\nspawn` split across
		// two of them, which a literal search would walk straight past.
		text := strings.Join(strings.Fields(string(content)), " ")
		for _, named := range bareCommandsIn(text, subcommands) {
			t.Errorf("the template's %s says %q, which only a standalone install can run: name the subcommand alone (%q)",
				path, "archimedes "+named, named)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Naming subcommands alone is only readable if something says what to put
// in front of them, and that has to be somewhere an operator reaches before
// the docs that rely on it.
func TestTheInstanceTemplateStatesBothInvocationsWhereAnOperatorMeetsThem(t *testing.T) {
	readme := templateFile(t, "README.md")

	if !strings.Contains(readme, "## "+invocationSection) {
		t.Errorf("the instance README has no %q section: the convention every other file follows is explained nowhere", invocationSection)
	}
	for _, form := range []string{"archimedes <subcommand>", "gh archimedes <subcommand>"} {
		if !strings.Contains(readme, form) {
			t.Errorf("the instance README does not show `%s`: a reader with that install has nothing telling them what to type", form)
		}
	}
}

// AGENTS.md is the sharpest case for all of this: its reader is a coding
// agent that will do as it is told, and an instance scaffolded by someone
// with only the extension install would otherwise send it to a binary that
// is not on the PATH. It points at the README section by name rather than
// restating it, which makes the name a link -- and one nothing else would
// notice going stale.
func TestTheInstanceTemplateSendsAnAgentToThatSectionRatherThanRestatingIt(t *testing.T) {
	if agents := templateFile(t, "AGENTS.md"); !strings.Contains(agents, invocationSection) {
		t.Errorf("the instance's AGENTS.md does not name the %q section: an agent reading it has nothing to follow", invocationSection)
	}
}

// invocationSection is the heading in the instance README that establishes
// the two forms, named here because two files agree on it.
const invocationSection = "Running a command"

func templateFile(t *testing.T, path string) string {
	t.Helper()
	content, err := fs.ReadFile(archimedes.Template(), path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
