package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

	subcommands := map[string]bool{}
	for _, sub := range root.Commands() {
		subcommands[sub.Name()] = true
	}

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
