#!/usr/bin/env bash
# A tag must not be able to publish binaries the tests haven't passed on.
#
# That guarantee lives entirely in two workflow files, and the way it fails
# is quiet: someone splits the check into a workflow that merely runs
# alongside the release instead of gating it, and every red build still
# ships. Nothing else in this suite would notice, so this file reads the
# workflows themselves and pins the shape the guarantee needs --
#
#   - the suites run on a push, so they run at all;
#   - the release job depends on that same job rather than a second copy of
#     it, so a green gate and a red suite cannot be different questions;
#   - CI drives the commands a developer drives, not a parallel recipe.
#
# It cannot prove GitHub honors `needs:`, which is not in doubt. It proves
# we asked for it.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
TEST_WF="$ROOT/.github/workflows/test.yml"
RELEASE_WF="$ROOT/.github/workflows/release.yml"

echo "the release is gated on the tests:"

assert_file_exists "$TEST_WF" "there is a workflow that runs the tests"
assert_file_exists "$RELEASE_WF" "there is a workflow that publishes a release"

test_wf="$(cat "$TEST_WF" 2>/dev/null)"
release_wf="$(cat "$RELEASE_WF" 2>/dev/null)"

# Runs on its own, without anyone remembering to.
assert_contains "$test_wf" "push:" "the test workflow runs on a push"
# ...and is callable, which is what lets the release depend on this exact
# job instead of restating it.
assert_contains "$test_wf" "workflow_call:" "the test workflow can be called by another workflow"

# The same two commands a developer runs in a checkout. A CI-only recipe
# here is a second suite that can pass while the real one fails.
assert_contains "$test_wf" "go test ./..." "CI runs the Go suite the same way a developer does"
assert_contains "$test_wf" "tests/run-all.sh" "CI runs the bash suite the same way a developer does"

# The gap CI leaves has to be stated where the gap is, not just known --
# and stated in prose, not by CI quietly setting the opt-in and making
# billed calls on every push. Matching the bare name would pass hardest in
# exactly that case, so only comment lines count.
gap_note="$(grep -E '^[[:space:]]*#.*ARCHIMEDES_TEST_LIVE_DRIVERS' "$TEST_WF")"
assert_contains "$gap_note" "ARCHIMEDES_TEST_LIVE_DRIVERS" \
  "the workflow says which tests it does not exercise"

# The commands the workflow runs are documented for a developer to run by
# hand, and the workflow cites that section as its source. Nothing else
# would notice the two drifting apart.
docs="$(cat "$ROOT/docs/cli.md" 2>/dev/null)"
assert_contains "$docs" "go test ./..." "the Go suite command is documented"
assert_contains "$docs" "./tests/run-all.sh" "the bash suite command is documented"

release_job="$(workflow_job_block "$RELEASE_WF" release)"
gate_job="$(workflow_job_block "$RELEASE_WF" test)"

assert_contains "$release_job" "cli/gh-extension-precompile" \
  "the release job is the one that publishes"
# The dependency, however it is spelled -- `needs: test`, `needs: [test]`,
# or a list on the lines below it. What matters is that the publishing job
# names the gate, not which YAML shape it names it in, so read the value
# rather than matching the line.
needs_value="$(printf '%s\n' "$release_job" | awk '
  /^ *needs:/ { inneeds = 1; sub(/^ *needs: */, ""); print; next }
  inneeds && /^ *- / { print; next }
  inneeds { inneeds = 0 }
')"
assert_contains "$needs_value" "test" "publishing waits on the test job"
assert_contains "$gate_job" "uses: ./.github/workflows/test.yml" \
  "the job it waits on is the same one a push runs, not a second recipe"

report
