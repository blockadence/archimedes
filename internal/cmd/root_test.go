package cmd

import (
	"io/fs"
	"path/filepath"
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

// assertNamesSubcommandsAlone fails when content tells its reader to type a
// command only one of the two installs provides. `what` names where it was
// found, for a message that points at the file to fix.
//
// The content is flattened to one line first: these files are hard-wrapped,
// so the mention this is most likely to meet is an `archimedes` and its
// subcommand split across two of them, which a literal search would walk
// straight past.
func assertNamesSubcommandsAlone(t *testing.T, what, content string, subcommands map[string]bool) {
	t.Helper()
	for _, named := range bareCommandsIn(strings.Join(strings.Fields(content), " "), subcommands) {
		t.Errorf("%s says %q, which only a standalone install can run: name the subcommand alone (%q)",
			what, "archimedes "+named, named)
	}
}

// An instance's own files are on the far side of the line the test above
// draws. What that one guards is what the tool *prints*, which can be built
// from invocation.Name() because it is composed fresh on the machine
// reading it. An instance's files are *written*: the commands that scaffold
// one copy and generate them into a repository that then commits them,
// edits them, and shares them with teammates and agents who may have either
// install or none. So they cannot name the invoking form -- and they cannot
// name one fixed form either, since half the readers have not got it.
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

// The check is a walk of an instance rather than a list of the code paths
// that write into one. Naming those was the earlier shape and it failed
// once already: the walk enumerated the embedded template, the dossier stub
// `bootstrap` writes was not a template file, and it told its readers to run
// `archimedes sync-house-rules` for months. A second hand-named walk fixed
// that one file and left the same gap in front of the third.
//
// So this reads the artifact -- everything scaffoldInstance produces --
// rather than the writers of it. What a subcommand nobody has thought about
// yet writes into an instance is covered by being in the instance, without
// anything here naming it.
func TestAScaffoldedInstanceNamesNoCommandHalfItsReadersHaventGot(t *testing.T) {
	subcommands := subcommandNames(newRootCmd())
	root, parent := scaffoldInstance(t)

	// What this tool writes into *other people's* repositories is exempt,
	// and holds one fixed form on purpose: keying a committed file to how
	// the operator who generated it happened to install would put a
	// spurious diff in every such repo the first time somebody with the
	// other install ran the sync. Those repositories are cloned as siblings
	// of the instance, so the whole of that exemption here is where the
	// read is rooted -- one directory higher and it would be reading them.
	// Stand one in, so that moving the root fails saying so rather than
	// quietly starting to police somebody else's repo.
	elsewhere := filepath.Join(parent, orgRepoName, "HOUSE_RULES.md")
	writeFile(t, elsewhere, "Refresh this file with `archimedes sync-house-rules`.\n")

	files := instanceFiles(t, root)

	for rel, content := range files {
		// Every file, `scaffolding/` included. Those are pushed into other
		// people's repositories and so stay fixed under either install,
		// which is a stricter rule than this one rather than a different
		// one -- there is nothing here for them to fail, and no reason to
		// carve them out.
		assertNamesSubcommandsAlone(t, "the instance's "+rel, content, subcommands)
	}

	sibling := filepath.Join(parent, orgRepoName) + string(filepath.Separator)
	for rel := range files {
		if strings.HasPrefix(filepath.Join(root, rel), sibling) {
			t.Errorf("%s was read as part of the instance, but it is beside one: "+
				"rooted there this polices repositories the tool only writes into", rel)
		}
	}

	// And the files the hand-named walks used to name were reached, plus
	// the one neither of them would have: a walk covering nothing must not
	// pass by finding nothing.
	for _, want := range []string{
		"README.md",
		filepath.Join("repos", orgRepoName+".md"),
		filepath.Join("work", workSlug, "status.md"),
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("the instance's %s was never read: this covers less of one than it looks like it does", want)
		}
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

// templateFile reads one file out of the embedded template. The two tests
// above are about the template rather than an instance: they are what makes
// the convention followable, and it has to be in the seed data every
// instance is created from rather than in any one instance.
func templateFile(t *testing.T, path string) string {
	t.Helper()
	content, err := fs.ReadFile(archimedes.Template(), path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
