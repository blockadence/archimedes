#!/usr/bin/env bash
# Something outside the release gate watches upstream for the next version
# bump, and what it opens is read before it is merged.
#
# tests/ci_action_runtimes.sh holds the workflows to a table of floors and
# cannot check that table against upstream, because a test in a release gate
# must not reach the network. .github/dependabot.yml is the half that can.
# This file pins its shape, and pins the one thing that makes it more than a
# notification: that the pull request it opens runs test.yml, and therefore
# runs the floors guard, before anyone reviews it.
#
# Three ways this arrangement goes quiet, and what is asserted against each:
#
#   - the config is deleted, or was never on the default branch, and the
#     repository is back to the manual cadence that lost once already;
#   - the scope widens -- a second ecosystem picked up because this file was
#     left open -- and the twelve reviews a year that were argued for become
#     something nobody agreed to pay;
#   - test.yml's `push` filter is narrowed to named branches, which takes the
#     floors guard off the bot's pull requests without touching the guard,
#     and is the one of the three that leaves everything still looking right.
#
# What it cannot assert is the failure mode the issue names: the floors table
# going stale behind a bot that keeps bumping past it. No file check can see
# that. What it can do is keep the two files pointed at each other and at the
# page that says which of them is load-bearing, so the next reader of either
# is told the other exists.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
DEPENDABOT="$ROOT/.github/dependabot.yml"
TEST_WF="$ROOT/.github/workflows/test.yml"
FLOORS="$ROOT/tests/ci_action_runtimes.sh"

echo "something watches upstream for the next bump:"

assert_file_exists "$DEPENDABOT" "there is a Dependabot configuration"

# Instructions only, everywhere below. That file explains itself at length
# and names, in prose, both the ecosystems it deliberately does not cover and
# the cadence it deliberately does not use -- so a whole-file grep for
# "gomod" or "weekly" is answered by the paragraph ruling it out, and a
# whole-file grep for "github-actions" would still pass with the config
# emptied out.
config="$(grep -vE '^[[:space:]]*#' "$DEPENDABOT" 2>/dev/null)"
# ...except where the question asked is about the prose itself, below.
config_prose="$(cat "$DEPENDABOT" 2>/dev/null)"

assert_contains "$config" "package-ecosystem: github-actions" \
  "it watches the actions the workflows use"

# Exactly one, which is the scoping the decision to adopt this rested on:
# monthly review of a handful of `uses:` lines. Another ecosystem is a
# separate argument and has to be made rather than inherited from this file
# already existing.
ecosystems="$(printf '%s\n' "$config" | grep -cE '^[[:space:]]*-?[[:space:]]*package-ecosystem:')"
assert_eq "$ecosystems" "1" "it watches that and nothing else"

schedule="$(printf '%s\n' "$config" | sed -n 's/^[[:space:]]*interval:[[:space:]]*//p')"
assert_eq "$schedule" "monthly" "it opens pull requests monthly rather than weekly"

# Grouped: a quiet month is one pull request and a busy one is still one,
# rather than one per action. The group has to be unrestricted to mean that
# -- a group with a narrower pattern list leaves everything it does not match
# arriving one pull request at a time, which is the cost this was scoped to
# avoid, spelled as configuration that looks like it was avoided.
assert_contains "$config" "groups:" "the bumps arrive grouped rather than one pull request per action"
group_patterns="$(printf '%s\n' "$config" | awk '
  /^[[:space:]]*patterns:/ { inpatterns = 1; next }
  inpatterns && /^[[:space:]]*-[[:space:]]*/ { sub(/^[[:space:]]*-[[:space:]]*/, ""); gsub(/"/, ""); print; next }
  inpatterns { inpatterns = 0 }
')"
assert_eq "$group_patterns" "*" "the group covers every action, so a busy month is still one pull request"

echo ""
echo "what it opens is run before it is read:"

# The mechanism, and it is the whole reason a bot bumping `uses:` lines is
# safe here: Dependabot pushes a branch to this repository -- named
# `dependabot/github_actions/<action>-<version>`, or `dependabot/github_
# actions/<group>-<hash>` for a grouped run -- and that push runs test.yml,
# which runs tests/run-all.sh, which runs the floors guard. A bump past a
# floor therefore fails on the bot's own pull request rather than after
# someone merges it.
#
# `branches: ["**"]` is what makes that true, and `**` is the pattern that
# matches a `/` -- a narrower filter, or a list of named branches, would run
# nothing on those pushes. Nothing else in this suite would notice, because
# every other guarantee about test.yml is about what happens once it runs.
#
# Asserted as the exact filter rather than by matching a branch name against
# whatever is there: this is bash, GitHub's filter patterns are not bash
# globs, and a half-right implementation of somebody else's matcher would be
# a worse thing to trust than a filter that has to be re-argued by hand if it
# is ever narrowed.
push_block="$(yaml_block "$TEST_WF" "  push:" | grep -vE '^[[:space:]]*#')"
branches="$(printf '%s\n' "$push_block" | sed -n 's/^[[:space:]]*branches:[[:space:]]*//p')"
assert_contains "$branches" '"**"' \
  "test.yml runs on a push to every branch, including the dependabot/github_actions/... ones the bot pushes"

echo ""
echo "the two halves are pointed at each other:"

# The bot says a newer version exists; the floors table says which versions
# someone has read an upstream action.yml for. Merging the first as if it
# were the second is the way this ends up back where issue 52 started, so
# each file has to name the other and the page has to say which is
# load-bearing. This is the most a file check can do about a table going
# stale: make sure nobody meets one half without being told about the other.
floors_header="$(grep -E '^[[:space:]]*#' "$FLOORS")"
assert_contains "$floors_header" ".github/dependabot.yml" \
  "the floors table names the bot that will propose bumps against it"
assert_contains "$config_prose" "tests/ci_action_runtimes.sh" \
  "the bot's configuration names the guard its pull requests have to clear"

docs="$(cat "$ROOT/docs/cli.md" 2>/dev/null)"
assert_contains "$docs" ".github/dependabot.yml" \
  "the docs name the configuration that watches upstream"
assert_contains "$docs" "tests/ci_watches_action_versions.sh" \
  "the docs name the test that holds that configuration to the scope it was argued for"

report
