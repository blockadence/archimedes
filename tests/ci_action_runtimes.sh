#!/usr/bin/env bash
# No action in .github/workflows/ may be on a release that targets Node 20.
#
# The runner was papering over the mismatch and saying so, on every run:
#
#   Node.js 20 is deprecated. The following actions target Node.js 20 but
#   are being forced to run on Node.js 24: ...
#
# "Forced" is the notice that the override goes away. When it does it goes
# away on test.yml first, which is release.yml's gate -- so the first thing
# that breaks is the ability to cut a release, for a reason that has nothing
# to do with the tag being pushed. This file is what keeps that from coming
# back on the next copy-paste from an older workflow.
#
# What it checks is a floor per action, not an exact version: what matters
# is that the release we name is not on the node20 side of that action's
# history, and pinning the exact current version here would turn every
# routine bump into a test edit.
#
# Where the floors come from, and how to redo one: an action declares its
# runtime in its own action.yml, under `runs: using:` -- the same field the
# runner reads to decide whether to force the override. The floor for an
# action is the lowest major whose action.yml says node24 --
#
#   gh api "repos/actions/checkout/contents/action.yml?ref=v5" \
#     --jq .content | base64 -d | grep -A3 '^runs:'
#
# -- run against each major until it flips.
#
# What this cannot prove, and it is the load-bearing half: that the table
# below is true. The floors are a reading of upstream taken by hand on
# 2026-09-07 and never re-checked from here, because a test in a gate must
# not reach the network to pass. So this proves the tree matches the table.
# That the table matches upstream was proved once, by the `gh api` above,
# and again by a real run reporting no deprecation annotation -- see
# docs/cli.md, "The Node the actions run on".
#
# What does reach the network is .github/dependabot.yml, which watches the
# same `uses:` lines from outside the gate and opens one grouped pull
# request a month when a newer release exists. It is not a second opinion on
# this table. It says a newer version exists; the table says which versions
# someone has read an upstream action.yml for -- and the pull request it
# opens runs this file before anyone reviews it, so a bump past a floor
# fails on arrival. What neither can see is a row here going stale: a floor
# is only as current as the reading it was taken from, and merging a green
# bump is not that reading. Do it in the same pull request when a bump
# lands, and move the date above with it.
#
# An action not in the table fails rather than passing quietly. That is the
# point: the table is the record of someone having looked, and a `uses:` no
# one has looked at is exactly the thing this file exists to catch.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
WORKFLOWS="$ROOT/.github/workflows"

# <action> <floor>, where the floor is the lowest major that does not target
# Node 20 -- or `composite`, for an action that ships no JavaScript of its
# own and so has no runtime to deprecate. `cli/gh-extension-precompile` is
# the composite one, and why it is left alone rather than bumped is in
# docs/cli.md rather than restated here.
NODE24_FLOORS="$(cat <<'EOF'
actions/checkout 5
actions/setup-go 6
actions/setup-node 5
astral-sh/setup-uv 7
cli/gh-extension-precompile composite
EOF
)"

floor_for() { # <action>
  printf '%s\n' "$NODE24_FLOORS" | awk -v want="$1" '$1 == want { print $2; exit }'
}

echo "no action in the workflows is on a Node 20 release:"

# `workflow_uses` is in helpers.sh with the other workflow readers, so this
# file's reading of a `uses:` line cannot drift from anyone else's.
lines="$(workflow_uses "$WORKFLOWS"/*.yml)"

if [ -z "$lines" ]; then
  fail "there is at least one action to check (found no uses: lines under $WORKFLOWS)"
  report
  exit
fi
pass "the workflows were read and name at least one action"

while IFS= read -r use; do
  [ -n "$use" ] || continue

  # A workflow calling another workflow in this repo, not an action. It has
  # no runtime of its own -- the jobs it defines are checked by this same
  # loop, because the file it points at is in the directory being read.
  case "$use" in
    ./*)
      assert_file_exists "$ROOT/${use#./}" "$use is a workflow in this repository"
      continue
      ;;
  esac

  action="${use%@*}"
  ref="${use##*@}"
  floor="$(floor_for "$action")"

  if [ -z "$floor" ]; then
    fail "$action has a recorded Node runtime (unknown action: read its action.yml \`runs: using:\` and add a row to NODE24_FLOORS)"
    continue
  fi

  if [ "$floor" = "composite" ]; then
    pass "$use is a composite action, so it has no Node runtime to be deprecated"
    continue
  fi

  # A version tag and nothing else. A commit SHA is the case worth naming:
  # `40b3ef1` would read as major 40 and clear every floor here, so this
  # file would go quiet on exactly the pin it cannot evaluate. SHA pinning
  # is deliberately not done in this repository (docs/cli.md); if that
  # changes, this is the assertion that has to change with it, rather than
  # the one that silently stops asking.
  case "$ref" in
    v[0-9]*)
      if ! printf '%s' "$ref" | grep -qE '^v[0-9]+(\.[0-9]+)*$'; then
        fail "$use names a plain version tag (got [$ref])"
        continue
      fi
      ;;
    *)
      fail "$use names a version tag rather than a SHA or branch (got [$ref]; this file reads majors, and cannot tell which release a SHA is)"
      continue
      ;;
  esac
  major="$(printf '%s' "$ref" | sed -nE 's/^v([0-9]+).*/\1/p')"

  if [ "$major" -ge "$floor" ]; then
    pass "$use is at or past $action@v$floor, the first release that does not target Node 20"
  else
    fail "$use targets Node 20 ($action does not leave Node 20 until v$floor)"
  fi
done <<< "$lines"

echo ""
echo "the answer is written down where a reader will look for it:"

# The same thing ci_gates_release.sh and ci_runs_live_drivers.sh ask of
# docs/cli.md, and for the same reason: someone deciding whether a bump is
# safe reads the page, not a YAML comment, and the page claiming this file
# holds the line is only true while the two are pointed at each other.
docs="$(cat "$ROOT/docs/cli.md" 2>/dev/null)"
assert_contains "$docs" "tests/ci_action_runtimes.sh" \
  "the docs name the test that holds the workflows off a deprecated Node"
assert_contains "$docs" "cli/gh-extension-precompile" \
  "the docs answer the one action here we do not control"

report
