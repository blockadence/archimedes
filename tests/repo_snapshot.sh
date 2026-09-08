#!/usr/bin/env bash
# Unit tests for the snapshot/restore helpers the drivers share
# (drivers/lib/repo-snapshot.sh). These are what let a driver that cannot
# help writing all over the target repo -- spec-kit unpacking a toolchain
# into it, pocock handing an agent session the run of it -- still honor the
# fixed-location contract's "no trace left behind" guarantee: snapshot the
# repo's state first, then afterwards undo everything the run added or
# changed, keeping only the declared fixed_path for the driver runner to
# harvest.
#
# No network, no CLIs, no spec-kit -- the helpers are exercised directly
# against a throwaway git repo with hand-made "scaffolding", so this runs in
# the normal suite rather than being opt-in like the live e2e tests.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

# The helpers need bash 4 for their associative arrays, and say so; the
# drivers that source them check for it before they touch anybody's repo.
# This file has to make the same check for itself, because the rest of the
# suite is deliberately written to run under the bash 3.2 macOS still ships
# -- and a file that errored out here rather than skipping would report the
# machine's bash as a broken helper.
if [ "${BASH_VERSINFO[0]}" -lt 4 ]; then
  echo "skip: repo_snapshot.sh (the snapshot/restore helpers need bash 4+, running ${BASH_VERSION}; the drivers that source them refuse under an older one too)"
  exit 77
fi

ROOT="$(cd "$HERE/.." && pwd)"
source "$ROOT/drivers/lib/repo-snapshot.sh"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
REPO="$WORK/repo"
mkdir -p "$REPO/src"
echo "# readme" > "$REPO/README.md"
echo "console.log('hi')" > "$REPO/src/index.js"
echo "doomed" > "$REPO/src/doomed.js"
make_repo_at "$REPO"

# State that was already there before the driver ran: one untracked file and
# one dirty tracked file. Neither belongs to the driver, so restore must
# leave both exactly as they are.
echo "mine" > "$REPO/scratch-note.md"
echo "# readme, edited by a human" > "$REPO/README.md"

echo "repo snapshot/restore:"

SNAP="$WORK/snapshot"
snapshot_repo_state "$REPO" > "$SNAP"

# Now simulate a spec-kit run: scaffolding written all over the repo, a
# tracked file clobbered, a tracked file deleted, and -- buried inside the
# scaffolding -- the one artifact we actually want to keep.
mkdir -p "$REPO/.specify/memory" "$REPO/.specify/scripts/bash" "$REPO/.claude/skills/speckit-constitution"
echo "the constitution" > "$REPO/.specify/memory/constitution.md"
echo "resolver" > "$REPO/.specify/scripts/bash/resolve-template.sh"
echo "feature.json" > "$REPO/.specify/.gitignore"
echo "skill" > "$REPO/.claude/skills/speckit-constitution/SKILL.md"
echo "clobbered by the toolchain" > "$REPO/src/index.js"
rm "$REPO/src/doomed.js"

restore_repo_state "$REPO" "$SNAP" ".specify/memory/constitution.md"

assert_file_exists "$REPO/.specify/memory/constitution.md" \
  "the kept path survives restore, so the driver runner still has something to harvest"
assert_eq "$(cat "$REPO/.specify/memory/constitution.md" 2>/dev/null)" "the constitution" \
  "the kept path's content is untouched by restore"

assert_file_missing "$REPO/.specify/scripts/bash/resolve-template.sh" \
  "scaffolding the run added is removed"
assert_file_missing "$REPO/.specify/.gitignore" \
  "scaffolding the run added is removed even when it is itself a gitignore file"
assert_file_missing "$REPO/.claude/skills/speckit-constitution/SKILL.md" \
  "scaffolding added outside the kept path's own directory tree is removed too"

assert_dir_missing "$REPO/.claude" "directories left empty by the cleanup are pruned"
assert_dir_missing "$REPO/.specify/scripts" \
  "empty directories are pruned all the way up, not just the leaf"
assert_dir_exists "$REPO/.specify/memory" \
  "a directory still holding the kept path is not pruned"

assert_eq "$(cat "$REPO/src/index.js" 2>/dev/null)" "console.log('hi')" \
  "a tracked file the run clobbered is restored from HEAD"
assert_file_exists "$REPO/src/doomed.js" \
  "a tracked file the run deleted is restored from HEAD"

assert_file_exists "$REPO/scratch-note.md" \
  "an untracked file that predates the run is left alone"
assert_eq "$(cat "$REPO/README.md" 2>/dev/null)" "# readme, edited by a human" \
  "a tracked file already dirty before the run keeps its edits (restore undoes the run's changes, not the human's)"

# The only thing standing between this repo and its pre-run git status is
# the artifact the driver runner is about to move out of it.
assert_eq "$(git -C "$REPO" status --porcelain)" \
  "$(printf ' M README.md\n?? .specify/\n?? scratch-note.md')" \
  "git status after restore shows the pre-run state plus the kept artifact, nothing else"

echo ""
echo "repo snapshot/restore, no kept path:"

REPO2="$WORK/repo2"
mkdir -p "$REPO2"
echo "# readme" > "$REPO2/README.md"
make_repo_at "$REPO2"

SNAP2="$WORK/snapshot2"
snapshot_repo_state "$REPO2" > "$SNAP2"
mkdir -p "$REPO2/.specify/memory"
echo "junk" > "$REPO2/.specify/memory/constitution.md"
restore_repo_state "$REPO2" "$SNAP2"

assert_eq "$(git -C "$REPO2" status --porcelain)" "" \
  "with nothing to keep, restore returns the repo to a completely clean git status"
assert_dir_missing "$REPO2/.specify" \
  "with nothing to keep, the scaffolding's directories are pruned entirely"

echo ""
echo "repo snapshot/restore, directories holding no files:"

# git tracks no directories at all, so an empty one a scaffolder leaves
# behind is invisible to `git status` -- it has to be caught by diffing the
# directory listing, or the "no trace" guarantee quietly isn't one.
REPO3="$WORK/repo3"
mkdir -p "$REPO3/keep-me/nested"
echo "# readme" > "$REPO3/README.md"
make_repo_at "$REPO3"

SNAP3="$WORK/snapshot3"
snapshot_repo_state "$REPO3" > "$SNAP3"
mkdir -p "$REPO3/.specify/extensions" "$REPO3/.specify/workflows/speckit"
restore_repo_state "$REPO3" "$SNAP3"

assert_dir_missing "$REPO3/.specify" \
  "a directory the run created and left holding nothing at all is removed"
assert_dir_exists "$REPO3/keep-me/nested" \
  "an empty directory that predates the run is left alone"
assert_eq "$(git -C "$REPO3" status --porcelain)" "" \
  "the repo is clean afterwards (which git status would have said either way -- hence the directory assertions above)"

echo ""
echo "repo snapshot/restore, naming what the run changed:"

# Restoring silently is right for a driver whose tool was always going to
# scaffold itself in. It is not right for one whose session was asked for a
# single file and wrote four: that driver has to tell the operator what
# happened, which means asking the same diff restore acts on to answer in
# words instead. One diff, two readings -- a second way of working out what
# a run touched would be free to disagree with the one that cleans up.
REPO5="$WORK/repo5"
mkdir -p "$REPO5/src"
echo "# readme" > "$REPO5/README.md"
echo "console.log('hi')" > "$REPO5/src/index.js"
make_repo_at "$REPO5"
echo "mine" > "$REPO5/scratch-note.md"

SNAP5="$WORK/snapshot5"
snapshot_repo_state "$REPO5" > "$SNAP5"

mkdir -p "$REPO5/docs/adr"
echo "the map" > "$REPO5/CONTEXT.md"
echo "an ADR nobody asked for" > "$REPO5/docs/adr/001-widgets.md"
echo "helpfully reformatted" > "$REPO5/src/index.js"
mkdir -p "$REPO5/.claude/skills"

changed="$(paths_changed_since_snapshot "$REPO5" "$SNAP5" CONTEXT.md)"

assert_contains "$changed" "docs/adr/001-widgets.md" \
  "a file the run added is named"
assert_contains "$changed" "src/index.js" \
  "a tracked file the run changed is named"
assert_not_contains "$changed" "CONTEXT.md" \
  "the kept path is not named -- writing it is what the run was for"
assert_not_contains "$changed" "scratch-note.md" \
  "an untracked file that predates the run is not named: it is not the run's doing"
assert_eq "$(printf '%s' "$changed" | wc -l | tr -d ' ')" "1" \
  "nothing else is named (two paths, so one newline between them)"

# What restore then does about them, on the same repo and the same
# snapshot: naming and undoing have to agree, because a driver that reports
# one set and cleans up another leaves the operator looking in the wrong
# place.
restore_repo_state "$REPO5" "$SNAP5" CONTEXT.md

assert_file_missing "$REPO5/docs/adr/001-widgets.md" "everything named is put back"
assert_dir_missing "$REPO5/docs" "and the directories it was written into go too"
assert_dir_missing "$REPO5/.claude" \
  "an empty directory the run left is pruned as well, though git status cannot see it and neither can the naming"
assert_eq "$(cat "$REPO5/src/index.js" 2>/dev/null)" "console.log('hi')" \
  "a tracked file the run changed is put back to what HEAD says"
assert_file_exists "$REPO5/CONTEXT.md" "the kept path survives"
assert_file_exists "$REPO5/scratch-note.md" "and so does what predated the run"

assert_eq "$(paths_changed_since_snapshot "$REPO5" "$SNAP5" CONTEXT.md)" "" \
  "after restore there is nothing left to name"

echo ""
echo "repo snapshot/restore, naming what the run changed when HEAD moved:"

# The same refusal restore makes, for the same reason: with HEAD moved, what
# the run added cannot be told apart from what was already committed, so
# there is no honest answer to give. Failing is what lets a driver say the
# repo needs looking at rather than report an empty list as "it wrote
# nothing".
REPO6="$WORK/repo6"
mkdir -p "$REPO6"
echo "# readme" > "$REPO6/README.md"
make_repo_at "$REPO6"
SNAP6="$WORK/snapshot6"
snapshot_repo_state "$REPO6" > "$SNAP6"
echo "an ADR nobody asked for" > "$REPO6/ADR.md"
git -C "$REPO6" add -A
git -C "$REPO6" commit -qm "committed it too"

if err="$(paths_changed_since_snapshot "$REPO6" "$SNAP6" 2>&1)"; then
  fail "naming what changed refuses when HEAD moved, rather than reporting nothing changed"
else
  pass "naming what changed refuses when HEAD moved, rather than reporting nothing changed"
fi
assert_contains "$err" "HEAD moved" "the refusal says what went wrong"
assert_contains "$err" "$REPO6" "the refusal names the repo that needs looking at by hand"

echo ""
echo "repo snapshot/restore, HEAD moved during the run:"

# Every judgement restore makes is relative to the commit HEAD pointed at
# when the snapshot was taken. An agent session that commits has made the
# scaffolding indistinguishable from the repo's own history -- and, worse,
# left `git status` reading clean. Restoring on that basis would be
# guesswork, so it refuses and says so.
REPO4="$WORK/repo4"
mkdir -p "$REPO4"
echo "# readme" > "$REPO4/README.md"
make_repo_at "$REPO4"

SNAP4="$WORK/snapshot4"
snapshot_repo_state "$REPO4" > "$SNAP4"
mkdir -p "$REPO4/.specify/memory"
echo "scaffolding" > "$REPO4/.specify/memory/constitution.md"
git -C "$REPO4" add -A
git -C "$REPO4" commit -qm "committed the scaffolding"

if err="$(restore_repo_state "$REPO4" "$SNAP4" 2>&1)"; then
  fail "restore refuses when HEAD moved during the run"
else
  pass "restore refuses when HEAD moved during the run"
fi
assert_contains "$err" "HEAD moved" \
  "the refusal says what went wrong rather than failing silently"
assert_contains "$err" "$REPO4" \
  "the refusal names the repo that needs looking at by hand"
assert_file_exists "$REPO4/.specify/memory/constitution.md" \
  "the refusal changes nothing -- unwinding a commit is the operator's call, not this helper's"

report
