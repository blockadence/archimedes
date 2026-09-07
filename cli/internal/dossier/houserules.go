// Package dossier reads an instance's per-repo dossiers (repos/<name>.md).
// A dossier is hand-maintained prose; only the sections Archimedes acts on
// are parsed here.
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
// Kept byte-identical to lib.sh's HOUSE_RULES_STUB_BODY; TestStubBodyMatchesLibSh
// fails if the two drift apart.
const houseRulesStubBody = "TBD. Mandated decisions that must be respected even if unusual — the kind of\n" +
	"thing a new contributor (or agent) would otherwise get wrong by using good\n" +
	"judgment. Kept separate from \"Known gotchas\" below: gotchas are surprising\n" +
	"facts about the repo, house rules are standing directives. Edit this section\n" +
	"only here — `sync-house-rules.sh` pushes a durable copy into the repo\n" +
	"itself, and `spawn.sh` injects an ephemeral copy into every worktree\n" +
	"spawned for it, so this dossier is the one place changes need to be made."

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
	if body == houseRulesStubBody {
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
