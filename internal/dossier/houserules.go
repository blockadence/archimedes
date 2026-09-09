// Package dossier reads an instance's per-repo dossiers (repos/<name>.md).
// A dossier is hand-maintained prose; only the sections Archimedes acts on
// are parsed here.
//
// Dir is where an instance keeps them, and the one place that says so.
// Everything else here takes that directory as a parameter, so a dossier
// can be read and written against a bare directory with no instance around
// it — see Dir for why that is worth the parameter.
package dossier

import (
	"os"
	"path/filepath"
	"strings"
)

// HouseRulesHeading marks the dossier section holding a repo's mandated
// house rules: standing directives that must be respected even where they
// look unusual, as distinct from "Known gotchas" (surprising facts).
const HouseRulesHeading = "## House rules"

// HouseRulesFileName is the file a repo's house rules are delivered as, in
// both directions of the dual delivery: the durable copy committed into the
// repo itself (internal/reposync) and the ephemeral per-worktree copy
// (internal/spawn). One name, so the two copies stay recognizably the same
// artifact.
const HouseRulesFileName = "HOUSE_RULES.md"

// houseRulesStubBody is the placeholder bootstrap writes into a fresh
// dossier. It must be recognized as "no rules recorded yet" rather than a
// real rule — otherwise a never-edited dossier would get its instructional
// boilerplate delivered as though it were an actual mandated rule.
//
// The two subcommands are named on their own, without the `archimedes` or
// `gh archimedes` that goes in front: a dossier is committed to the
// instance and read by teammates and agents who may have either install, so
// naming one form would hand half of them a command they have not got. The
// instance README says once, under "Running a command", what to type in
// front of a subcommand. See docs/cli.md, "What an instance's own docs
// name".
const houseRulesStubBody = "TBD. Mandated decisions that must be respected even if unusual — the kind of\n" +
	"thing a new contributor (or agent) would otherwise get wrong by using good\n" +
	"judgment. Kept separate from \"Known gotchas\" below: gotchas are surprising\n" +
	"facts about the repo, house rules are standing directives. Edit this section\n" +
	"only here — `sync-house-rules` pushes a durable copy into the repo itself,\n" +
	"and `spawn` injects an ephemeral copy into every worktree spawned for it, so\n" +
	"this dossier is the one place changes need to be made."

// retiredStubBodies are placeholders earlier versions wrote, kept
// recognizable so an instance scaffolded back then still reads as having no
// house rules recorded. Reworded boilerplate is still boilerplate, and the
// cost of forgetting one is this package's worst failure: instructional
// text committed into someone's repo as a mandated rule. An entry only ever
// leaves this list when no dossier anywhere can still be carrying it, which
// is not a thing that can be known — so in practice they stay.
var retiredStubBodies = []string{
	// Before the vendored bash scripts were retired, the stub named them
	// rather than the subcommands that replaced them.
	"TBD. Mandated decisions that must be respected even if unusual — the kind of\n" +
		"thing a new contributor (or agent) would otherwise get wrong by using good\n" +
		"judgment. Kept separate from \"Known gotchas\" below: gotchas are surprising\n" +
		"facts about the repo, house rules are standing directives. Edit this section\n" +
		"only here — `sync-house-rules.sh` pushes a durable copy into the repo\n" +
		"itself, and `spawn.sh` injects an ephemeral copy into every worktree\n" +
		"spawned for it, so this dossier is the one place changes need to be made.",

	// Before the stub named the subcommands on their own, it prefixed them
	// with `archimedes` — the standalone install's form, and not a command
	// an operator who installed the gh extension has.
	"TBD. Mandated decisions that must be respected even if unusual — the kind of\n" +
		"thing a new contributor (or agent) would otherwise get wrong by using good\n" +
		"judgment. Kept separate from \"Known gotchas\" below: gotchas are surprising\n" +
		"facts about the repo, house rules are standing directives. Edit this section\n" +
		"only here — `archimedes sync-house-rules` pushes a durable copy into the\n" +
		"repo itself, and `archimedes spawn` injects an ephemeral copy into every\n" +
		"worktree spawned for it, so this dossier is the one place changes need to\n" +
		"be made.",
}

// isStub reports whether body is a placeholder nobody has filled in — the
// one bootstrap writes today, or one it wrote in the past.
func isStub(body string) bool {
	if body == houseRulesStubBody {
		return true
	}
	for _, retired := range retiredStubBodies {
		if body == retired {
			return true
		}
	}
	return false
}

// Path is a repo's dossier file inside an instance.
func Path(dossierDir, repo string) string {
	return filepath.Join(dossierDir, repo+".md")
}

// HouseRules returns the body of repo's dossier "## House rules" section —
// the single source of truth both the durable copy (sync-house-rules) and
// the ephemeral per-worktree copy (spawn) read from, so editing the dossier
// is the only place a house rule ever needs to change.
//
// Leading and trailing blank lines are trimmed. The result is empty, with
// no error, when the repo has no dossier, no such section, or a section
// still holding the unedited stub placeholder — all of which mean "no house
// rules recorded".
func HouseRules(dossierDir, repo string) (string, error) {
	data, err := os.ReadFile(Path(dossierDir, repo))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	body := section(string(data), HouseRulesHeading)
	if isStub(body) {
		return "", nil
	}
	return body, nil
}

// section returns the lines under heading, up to the next "## " heading,
// with surrounding blank lines trimmed.
func section(doc, heading string) string {
	var body []string
	found := false
	for _, line := range strings.Split(doc, "\n") {
		if !found {
			found = line == heading
			continue
		}
		if strings.HasPrefix(line, "## ") {
			break
		}
		body = append(body, line)
	}

	for len(body) > 0 && body[0] == "" {
		body = body[1:]
	}
	for len(body) > 0 && body[len(body)-1] == "" {
		body = body[:len(body)-1]
	}

	return strings.Join(body, "\n")
}
