#!/usr/bin/env bash
# The two live driver tests are run by the machine, on a cadence, and the
# credential they need cannot be reached from a fork.
#
# ci_gates_release.sh pins the shape of the free gate; this pins the shape
# of the billed one, and the two properties that make a billed one safe to
# have at all:
#
#   - it is never triggered by a pull request, so a fork cannot open one
#     that runs a job holding ANTHROPIC_API_KEY;
#   - the push check and the release gate stay free, so the billed suite
#     is not quietly a tax on every push.
#
# It also pins the discoverability the skip depends on: the message a
# skipped live test prints has to name the workflow that does run it, or
# the next reader of a "skipped" line is back where issue 34 started.
#
# Like ci_gates_release.sh, this reads the workflow files rather than
# GitHub. It cannot prove Actions honors `on:`; it proves we asked for it.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
LIVE_WF="$ROOT/.github/workflows/live-drivers.yml"
TEST_WF="$ROOT/.github/workflows/test.yml"
RELEASE_WF="$ROOT/.github/workflows/release.yml"

# The `on:` block alone, comments stripped. Read narrowly rather than
# reading the whole file, because both questions asked of it -- which
# triggers are there, and which are not -- are answered wrongly by a file
# that merely mentions a trigger in a comment explaining why it is absent.
on_block() { # <workflow-file>
  yaml_block "$1" "on:" | grep -vE '^[[:space:]]*#'
}

echo "the live driver tests are run by the machine:"

assert_file_exists "$LIVE_WF" "there is a workflow that runs the live driver tests"
# Comments and instructions, kept apart. This workflow's header explains
# what it runs and what it spends, so a whole-file grep for
# "tests/pocock-driver-e2e.sh" is answered by the paragraph about it rather
# than by the step that runs it -- an assertion that passes with the step
# deleted. The rule the free-checks section applies to test.yml applies
# here too, and for the same reason.
live_wf="$(grep -vE '^[[:space:]]*#' "$LIVE_WF" 2>/dev/null)"
live_triggers="$(on_block "$LIVE_WF" 2>/dev/null)"

assert_contains "$live_triggers" "schedule:" "the live suite runs on a schedule, not when someone remembers"
assert_contains "$live_triggers" "cron:" "the schedule names a cadence"
assert_contains "$live_triggers" "workflow_dispatch:" "and it can be run by hand before a release"

# The opt-in the two files sit behind. Without it they exit 77 and the
# workflow would be a green build that ran nothing -- the exact failure
# issue 34 exists to close.
assert_contains "$live_wf" "ARCHIMEDES_TEST_LIVE_DRIVERS: \"1\"" \
  "the live workflow sets the opt-in the two files sit behind"
assert_contains "$live_wf" "tests/pocock-driver-e2e.sh" "it runs the pocock live test"
assert_contains "$live_wf" "tests/spec-kit-driver-e2e.sh" "it runs the spec-kit live test"
assert_contains "$live_wf" "secrets.ANTHROPIC_API_KEY" \
  "it supplies the credential those tests need, out of a secret"

# A red build nobody owns is its own kind of noise, and a scheduled one is
# the easiest kind to stop seeing. Something has to land in front of a
# person.
assert_contains "$live_wf" "if: failure()" "a failure does something rather than only turning a square red"
assert_contains "$live_wf" "gh issue" "the something is an issue, which has an owner and outlives an email"

# What it costs and how often that is paid, on the file that spends it.
if grep -qE '^[[:space:]]*#.*\$[0-9]' "$LIVE_WF" 2>/dev/null; then
  pass "the workflow states what a run costs, in money"
else
  fail "the workflow states what a run costs, in money (no dollar figure in its comments)"
fi

echo ""
echo "the credential cannot be reached from a fork:"

# The whole defense. `pull_request` from a fork runs with the base repo's
# workflow file, so a job here that held the key would hand it to anything
# anyone opened a PR from. There is no safe spelling of this trigger for
# this workflow, so the assertion is that it does not appear at all.
assert_not_contains "$live_triggers" "pull_request" \
  "the live workflow is not triggered by a pull request, from a fork or otherwise"
# Scoped to the job that spends it, so adding a second job to this file
# later does not silently widen who holds the key.
assert_contains "$live_wf" "environment:" \
  "the credential is held by a deployment environment rather than handed to the whole repo"

echo ""
echo "the free checks stay free:"

release_wf="$(grep -vE '^[[:space:]]*#' "$RELEASE_WF" 2>/dev/null)"

# Comments are allowed to name both -- test.yml's whole point is saying
# what it does not do -- so read only the lines that are instructions to
# the runner. CI quietly setting the opt-in would put a billed call on
# every push, which is the one thing issue 34 ruled out.
test_steps="$(grep -vE '^[[:space:]]*#' "$TEST_WF")"
assert_not_contains "$test_steps" "ARCHIMEDES_TEST_LIVE_DRIVERS" \
  "the push check does not set the live-driver opt-in"
assert_not_contains "$test_steps" "ANTHROPIC_API_KEY" "the push check needs no credential"
assert_not_contains "$release_wf" "live-drivers.yml" \
  "publishing a tag does not wait on a billed suite or a third-party API being up"

echo ""
echo "what it costs is written down where an operator reads it:"

# The workflow's own header states the money; the docs have to state it too,
# along with the cadence and where the key lives, because an operator
# deciding whether this schedule is worth keeping reads docs/cli.md and not
# a YAML comment.
docs="$(cat "$ROOT/docs/cli.md" 2>/dev/null)"
assert_contains "$docs" "live-drivers.yml" "the docs name the workflow that runs the live tests"
assert_contains "$docs" "ANTHROPIC_API_KEY" "the docs say which credential it needs"
# A dollar amount, not merely the word cost.
if grep -qE '\$[0-9]' "$ROOT/docs/cli.md" 2>/dev/null; then
  pass "the docs put a number on what the schedule spends"
else
  fail "the docs put a number on what the schedule spends (no dollar figure on the page)"
fi

echo ""
echo "the skip says where the real thing is exercised:"

# The line a developer actually reads. `run-all.sh` names every skipped
# file; each of those files has to say where it is not being skipped.
#
# Every skip line, not the first one: these files skip for more than one
# reason -- no opt-in, no claude, no specify -- and whichever line a reader
# happens to hit is the one that has to carry the pointer. Checking only
# the first is how one of them stayed silent.
for f in pocock-driver-e2e.sh spec-kit-driver-e2e.sh; do
  skip_lines="$(grep -E '^[[:space:]]*echo "skip: ' "$ROOT/tests/$f")"
  silent=""
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    case "$line" in
      *live-drivers*) ;;
      *) silent="$silent$line"$'\n' ;;
    esac
  done <<< "$skip_lines"
  assert_eq "$silent" "" "every skip message in $f names the workflow that does run it"
done

# And the pocock skip has a second thing to point at, because it is the
# file that used to be the only test of drivers/pocock/run.sh at all.
pocock_header="$(sed -n '1,30p' "$ROOT/tests/pocock-driver-e2e.sh")"
assert_contains "$pocock_header" "pocock_driver_run.sh" \
  "the pocock live test points at the stub-level test that covers its orchestration for free"

echo ""
echo "the gap the push check reports is the gap that is left:"

# test.yml's comment about what it does not exercise was accurate when the
# two files were run by hand and nothing else. It has to say what is true
# now, or it becomes the misleading half of an honest report.
gap_note="$(grep -E '^[[:space:]]*#' "$TEST_WF")"
assert_contains "$gap_note" "live-drivers.yml" \
  "the push check's note about what it skips names where those files do run"

report
