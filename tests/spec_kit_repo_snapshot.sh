#!/usr/bin/env bash
# Unit tests for the spec-kit driver's snapshot/restore helpers
# (drivers/spec-kit/repo-snapshot.sh). These are what let a driver that has
# to scaffold a whole toolchain into the target repo still honor the
# fixed-location contract's "no trace left behind" guarantee: snapshot the
# repo's state first, then afterwards undo everything the run added or
# changed, keeping only the declared fixed_path for the driver runner to
# harvest.
#
# No network, no CLIs, no spec-kit -- the helpers are exercised directly
# against a throwaway git repo with hand-made "scaffolding", so this runs in
# the normal suite rather than being opt-in like the live e2e test.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
source "$ROOT/drivers/spec-kit/repo-snapshot.sh"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
REPO="$WORK/repo"
mkdir -p "$REPO/src"
(
  cd "$REPO"
  git init -q
  echo "# readme" > README.md
  echo "console.log('hi')" > src/index.js
  echo "doomed" > src/doomed.js
  git add -A
  git -c user.email=test@example.com -c user.name=test commit -qm init
)

# State that was already there before the driver ran: one untracked file and
# one dirty tracked file. Neither belongs to the driver, so restore must
# leave both exactly as they are.
echo "mine" > "$REPO/scratch-note.md"
echo "# readme, edited by a human" > "$REPO/README.md"

echo "spec-kit driver repo snapshot/restore:"

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
echo "spec-kit driver repo snapshot/restore, no kept path:"

REPO2="$WORK/repo2"
mkdir -p "$REPO2"
(
  cd "$REPO2"
  git init -q
  echo "# readme" > README.md
  git add -A
  git -c user.email=test@example.com -c user.name=test commit -qm init
)

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
echo "spec-kit driver repo snapshot/restore, directories holding no files:"

# git tracks no directories at all, so an empty one a scaffolder leaves
# behind is invisible to `git status` -- it has to be caught by diffing the
# directory listing, or the "no trace" guarantee quietly isn't one.
REPO3="$WORK/repo3"
mkdir -p "$REPO3/keep-me/nested"
(
  cd "$REPO3"
  git init -q
  echo "# readme" > README.md
  git add -A
  git -c user.email=test@example.com -c user.name=test commit -qm init
)

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
echo "spec-kit driver repo snapshot/restore, HEAD moved during the run:"

# Every judgement restore makes is relative to the commit HEAD pointed at
# when the snapshot was taken. An agent session that commits has made the
# scaffolding indistinguishable from the repo's own history -- and, worse,
# left `git status` reading clean. Restoring on that basis would be
# guesswork, so it refuses and says so.
REPO4="$WORK/repo4"
mkdir -p "$REPO4"
(
  cd "$REPO4"
  git init -q
  echo "# readme" > README.md
  git add -A
  git -c user.email=test@example.com -c user.name=test commit -qm init
)

SNAP4="$WORK/snapshot4"
snapshot_repo_state "$REPO4" > "$SNAP4"
mkdir -p "$REPO4/.specify/memory"
echo "scaffolding" > "$REPO4/.specify/memory/constitution.md"
(
  cd "$REPO4"
  git add -A
  git -c user.email=test@example.com -c user.name=test commit -qm "committed the scaffolding"
) >/dev/null

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
