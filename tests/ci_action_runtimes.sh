#!/usr/bin/env bash
# No action in .github/workflows/ may be on a release that targets Node 20.
#
# The runner is currently papering over the mismatch and saying so:
#
#   Node.js 20 is deprecated. The following actions target Node.js 20 but
#   are being forced to run on Node.js 24: ...
#
# "Forced" is the notice that the override goes away. When it does it goes
# away on test.yml first, which is release.yml's gate -- so the first thing
# that breaks is the ability to cut a release, for a reason that has nothing
# to do with the tag being pushed. This file is what keeps that from being
# re-introduced by a copy-paste from an older workflow.
#
# What it checks is a floor per action, not an exact version: what matters
# is that the release we name is not on the node20 side of that action's
# history, and pinning the exact current version here would turn every
# routine bump into a test edit.
#
# Where the floors come from, and how to redo one: an action declares its
# runtime in its own action.yml, under `runs: using:`, which is the same
# field the runner reads to decide whether to force the override. So the
# floor for an action is the lowest major whose action.yml says node24 --
#
#   gh api "repos/actions/checkout/contents/action.yml?ref=v5" \
#     --jq .content | base64 -d | grep -A3 '^runs:'
#
# -- run against each major until it flips. Checked 2026-09-07 against the
# floating major tags; the answers are recorded below so that a reader is
# not left to guess which of them was the boundary.
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
# own and so has no runtime to deprecate.
#
# cli/gh-extension-precompile is the one we do not control, and it is the
# one that needed answering rather than bumping. It is a composite action:
# `runs: using: composite`, a list of steps, no node entry point. It cannot
# be on Node 20 because it is not on Node at all -- which is why the
# annotation on release.yml named only actions/checkout and never named
# this. What it *does* carry is two nested actions, and at v2.2.0 those are
# pinned by SHA to actions/setup-go v6.4.0 and
# actions/attest-build-provenance v4.1.0, both node24 (the latter is itself
# composite over actions/attest v4.1.0, which is node24). So the answer for
# the third-party action is: unaffected, no bump, and no reading of its
# changelog for changes to generate_attestations or draft_release, because
# the version in the tree does not move.
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

# Every `uses:` the runner will act on, across every workflow. Comments are
# stripped first: these files explain themselves at length, and a paragraph
# naming an action is not an instruction to run it.
uses_lines() {
  grep -hE '^[[:space:]]*(-[[:space:]]*)?uses:' "$WORKFLOWS"/*.yml \
    | grep -vE '^[[:space:]]*#' \
    | sed -E 's/^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]*//; s/[[:space:]]*(#.*)?$//' \
    | sort -u
}

echo "no action in the workflows is on a Node 20 release:"

lines="$(uses_lines)"

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

  major="$(printf '%s' "$ref" | sed -nE 's/^v?([0-9]+).*/\1/p')"
  if [ -z "$major" ]; then
    fail "$use names a version this can read (expected a vN or vN.N.N tag, got [$ref])"
    continue
  fi

  if [ "$major" -ge "$floor" ]; then
    pass "$use is at or past $action@v$floor, the first release that does not target Node 20"
  else
    fail "$use targets Node 20 ($action does not leave Node 20 until v$floor)"
  fi
done <<< "$lines"

report
