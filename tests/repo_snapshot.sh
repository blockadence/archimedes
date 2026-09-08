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

echo ""
echo "repo snapshot/restore, work the repo already had uncommitted:"

# The hole the record-by-name snapshot could not see. A repo somebody is
# working in is normally dirty -- an untracked note, an edited source file --
# and the drivers are pointed at exactly those repos. Both kinds of path are
# recorded by name, so a run's write to one used to be indistinguishable from
# the state that predated it: the session clobbered the operator's work, the
# cleanup left it alone because leaving it alone is what the name says to do,
# and nothing anywhere said so.
#
# Unrestored it stays -- nothing here keeps a copy of what an uncommitted file
# said, and that is a deliberate cost not paid. Unreported it does not.
REPO7="$WORK/repo7"
mkdir -p "$REPO7/src"
echo "# readme" > "$REPO7/README.md"
echo "console.log('hi')" > "$REPO7/src/index.js"
echo "console.log('bye')" > "$REPO7/src/other.js"
printf 'node_modules/\n' > "$REPO7/.gitignore"
make_repo_at "$REPO7"

# What the operator had in flight when the run started: two untracked files,
# two edits to tracked ones, an older copy of the very file the run is for,
# and something under an ignored directory.
echo "notes to self" > "$REPO7/scratch-note.md"
echo "half a thought" > "$REPO7/half-done.md"
echo "an old map" > "$REPO7/CONTEXT.md"
echo "// mine, uncommitted" >> "$REPO7/src/index.js"
echo "// also mine" >> "$REPO7/src/other.js"
mkdir -p "$REPO7/node_modules/pkg"
echo "module.exports = 1" > "$REPO7/node_modules/pkg/index.js"

SNAP7="$WORK/snapshot7"
snapshot_repo_state "$REPO7" > "$SNAP7"

# The session, having been asked for one file.
echo "the map" > "$REPO7/CONTEXT.md"
echo "helpfully rewritten" > "$REPO7/scratch-note.md"
echo "console.log('reformatted')" > "$REPO7/src/index.js"
rm "$REPO7/half-done.md"
echo "module.exports = 2" > "$REPO7/node_modules/pkg/index.js"

changed7="$(paths_changed_since_snapshot "$REPO7" "$SNAP7" CONTEXT.md)"

assert_contains "$changed7" "scratch-note.md" \
  "an untracked file the operator had not committed, whose contents the run wrote over, is named"
assert_contains "$changed7" "src/index.js" \
  "a tracked file the operator had already edited, whose contents the run wrote over, is named"
assert_contains "$changed7" "half-done.md" \
  "and one the run removed outright is named too -- removing it is the same loss as writing over it"
assert_not_contains "$changed7" "src/other.js" \
  "an uncommitted edit the run left alone is not named: being dirty is not the same as being written to"
assert_not_contains "$changed7" "CONTEXT.md" \
  "the kept path is not named even though the operator had uncommitted work in it -- writing that one is what the run was for"
assert_not_contains "$changed7" "node_modules" \
  "a path git is ignoring is not named: fingerprinting those would mean reading the whole of node_modules on every run, which is the cost this deliberately does not pay"

# What restore then does about them, on the same repo and the same snapshot.
# It cannot put any of them back, so the only thing left that is worth doing
# is saying which ones -- and saying it here rather than leaving it to the
# caller, because this runs from a driver's exit trap, where the caller is a
# script on its way out and nothing else is going to ask.
restore7="$(restore_repo_state "$REPO7" "$SNAP7" CONTEXT.md 2>&1 >/dev/null)"

assert_contains "$restore7" "scratch-note.md" \
  "restore says which of the operator's uncommitted files it could not put back"
assert_contains "$restore7" "src/index.js" \
  "including the tracked one, which HEAD could only restore by throwing the operator's own edit away as well"
assert_contains "$restore7" "$REPO7" "and names the repo they are in"
assert_eq "$(cat "$REPO7/scratch-note.md" 2>/dev/null)" "helpfully rewritten" \
  "and leaves them as the run left them rather than guessing at what they said"
assert_eq "$(cat "$REPO7/src/index.js" 2>/dev/null)" "console.log('reformatted')" \
  "the tracked one included -- restoring it from HEAD would undo the operator's edit too"
assert_file_missing "$REPO7/half-done.md" \
  "and one the run removed stays removed, for the same reason: there is no copy of it"
assert_eq "$(cat "$REPO7/src/other.js" 2>/dev/null)" \
  "$(printf "console.log('bye')\n// also mine")" \
  "an uncommitted edit the run left alone is still exactly as the operator left it"
assert_file_exists "$REPO7/node_modules/pkg/index.js" \
  "and an ignored path is still there -- unreported, but not deleted either"

# The other half of the report: a run that touched none of it says nothing,
# so the message means something when it does appear.
REPO9="$WORK/repo9"
mkdir -p "$REPO9"
echo "# readme" > "$REPO9/README.md"
make_repo_at "$REPO9"
echo "notes to self" > "$REPO9/scratch-note.md"
echo "# readme, edited by a human" > "$REPO9/README.md"
SNAP9="$WORK/snapshot9"
snapshot_repo_state "$REPO9" > "$SNAP9"
echo "the map" > "$REPO9/CONTEXT.md"

assert_eq "$(paths_changed_since_snapshot "$REPO9" "$SNAP9" CONTEXT.md)" "" \
  "a run that wrote only what it was asked for names nothing, though the repo was dirty throughout"
assert_eq "$(restore_repo_state "$REPO9" "$SNAP9" CONTEXT.md 2>&1 >/dev/null)" "" \
  "and restore says nothing either -- the report has to be silent when there is nothing to report, or it is noise"

echo ""
echo "repo snapshot/restore, a run that staged what the repo already had:"

# A session running the one git command that does not move HEAD, so nothing
# above refuses. Before this, the staged path read as a tracked file gone
# dirty, HEAD had never heard of it, and restore's answer to that is to
# unstage it and delete it -- these helpers destroying the very uncommitted
# work they exist to leave alone.
REPO10="$WORK/repo10"
mkdir -p "$REPO10"
echo "# readme" > "$REPO10/README.md"
make_repo_at "$REPO10"
echo "notes to self" > "$REPO10/scratch-note.md"
SNAP10="$WORK/snapshot10"
snapshot_repo_state "$REPO10" > "$SNAP10"
echo "the map" > "$REPO10/CONTEXT.md"
git -C "$REPO10" add scratch-note.md

changed10="$(paths_changed_since_snapshot "$REPO10" "$SNAP10" CONTEXT.md)"
assert_contains "$changed10" "scratch-note.md" \
  "a file the run staged is named: the run was told to run no git command that changes the repo"

restore_repo_state "$REPO10" "$SNAP10" CONTEXT.md

assert_file_exists "$REPO10/scratch-note.md" \
  "restore does not delete a file that predated the run just because the run staged it"
assert_eq "$(cat "$REPO10/scratch-note.md" 2>/dev/null)" "notes to self" \
  "and leaves its contents alone -- the file is the operator's, only the index entry was the run's doing"
assert_eq "$(git -C "$REPO10" status --porcelain)" "$(printf '?? CONTEXT.md\n?? scratch-note.md')" \
  "the index entry the run added is undone, so the repo is dirty in exactly the way it was, plus the kept path"

echo ""
echo "repo snapshot/restore, paths git cannot be handed a line at a time:"

# The fingerprinting asks git for every path in one go, which means handing it
# a list one path per line -- and git reads that list the way it reads any
# line: a newline ends a path early, a trailing carriage return is stripped off
# it, and anything arriving quoted is unquoted. The last two are the ones worth
# a test, because git then answers for a *different* file and exits zero, so
# nothing downstream has any reason to doubt it. Given a sibling with the name
# git resolves to, the wrong hash lands on the right path and the report is
# quietly wrong in both directions at once.
REPO11="$WORK/repo11"
mkdir -p "$REPO11"
echo "# readme" > "$REPO11/README.md"
make_repo_at "$REPO11"

CR_NAME="$(printf 'note\r')"
printf 'the one with the carriage return\n' > "$REPO11/$CR_NAME"
printf 'the sibling git resolves that name to\n' > "$REPO11/note"
printf 'the one that looks quoted\n' > "$REPO11/\"quoted\".md"
printf 'the one with a newline in it\n' > "$REPO11/$(printf 'two\nlines.md')"

SNAP11="$WORK/snapshot11"
snapshot_repo_state "$REPO11" > "$SNAP11"

# Only the carriage-return one is written to. Its plain sibling is left alone,
# so a fingerprint that had been taken from the sibling reports the reverse of
# what happened: the file that changed looks untouched and the one that did not
# looks written over.
printf 'rewritten by the session\n' > "$REPO11/$CR_NAME"

changed11="$(changed_since_snapshot "$REPO11" "$SNAP11" | tr '\0' '\n')"

assert_contains "$changed11" "$CR_NAME" \
  "a path whose name ends in a carriage return is fingerprinted as itself, so a write to it is reported"
# Line-exact, because the record for the carriage-return path *starts* with
# the sibling's whole name -- which is the entire trouble -- so anything less
# than a whole-line match would be satisfied by the very record under test.
if printf '%s\n' "$changed11" | grep -qxF "$(printf 'O\tnote')"; then
  fail "its plain-named sibling is not reported in its place, which is the name git resolves when it reads the path a line at a time"
else
  pass "its plain-named sibling is not reported in its place, which is the name git resolves when it reads the path a line at a time"
fi
assert_eq "$(printf '%s\n' "$changed11" | grep -c "^O$(printf '\t')")" "1" \
  "exactly one path is reported written over, so no second record was invented for a name that only looked like one"
assert_not_contains "$changed11" '"quoted".md' \
  "a path that arrives looking quoted is left alone when it is left alone, rather than answered for by the name inside the quotes"
assert_not_contains "$changed11" "lines.md" \
  "and one with a newline in its name is not reported either"

echo ""
echo "repo snapshot, a repo with no commits yet, under a driver's shell options:"

# Every driver that sources this file runs under `set -euo pipefail`, and the
# fingerprinting happens inside a pipeline. A helper in there that hands back a
# non-zero status because there is no HEAD to diff against would not be a
# missing record -- pipefail and errexit would take the whole run down before
# the session ever started, on nothing worse than a repo whose first commit has
# not been made.
REPO12="$WORK/repo12"
mkdir -p "$REPO12"
git init -q "$REPO12"
echo "started, not committed" > "$REPO12/scratch-note.md"

if ( set -euo pipefail; snapshot_repo_state "$REPO12" > "$WORK/snapshot12" ); then
  pass "snapshotting a repo with no commits yet succeeds under the shell options every driver sets"
else
  fail "snapshotting a repo with no commits yet succeeds under the shell options every driver sets"
fi
assert_contains "$(tr '\0' '\n' < "$WORK/snapshot12")" "scratch-note.md" \
  "and it still fingerprints the work already sitting there, HEAD or no HEAD"

report
