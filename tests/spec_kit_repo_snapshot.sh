#!/usr/bin/env bash
# Unit tests for the spec-kit driver's snapshot/restore helpers
# (drivers/spec-kit/repo-snapshot.sh). These are what let a driver that has
# to scaffold a whole toolchain into the target repo still honor the
# fixed-location contract's "no trace left behind" guarantee: snapshot the
# repo's state first, then afterwards undo everything the run added or
# changed, keeping only the declared fixed_path for run-driver.sh to
# harvest.
#
# No network, no CLIs, no spec-kit -- the helpers are exercised directly
# against a throwaway git repo with hand-made "scaffolding", so this runs in
# the normal suite rather than being opt-in like the live e2e test.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
source "$ROOT/template/drivers/spec-kit/repo-snapshot.sh"

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
  "the kept path survives restore, so run-driver.sh still has something to harvest"
assert_eq "$(cat "$REPO/.specify/memory/constitution.md" 2>/dev/null)" "the constitution" \
  "the kept path's content is untouched by restore"

assert_file_missing "$REPO/.specify/scripts/bash/resolve-template.sh" \
  "scaffolding the run added is removed"
assert_file_missing "$REPO/.specify/.gitignore" \
  "scaffolding the run added is removed even when it is itself a gitignore file"
assert_file_missing "$REPO/.claude/skills/speckit-constitution/SKILL.md" \
  "scaffolding added outside the kept path's own directory tree is removed too"

[ -d "$REPO/.claude" ] \
  && fail "directories left empty by the cleanup are pruned" \
  || pass "directories left empty by the cleanup are pruned"
[ -d "$REPO/.specify/scripts" ] \
  && fail "empty directories are pruned all the way up, not just the leaf" \
  || pass "empty directories are pruned all the way up, not just the leaf"
[ -d "$REPO/.specify/memory" ] \
  && pass "a directory still holding the kept path is not pruned" \
  || fail "a directory still holding the kept path is not pruned"

assert_eq "$(cat "$REPO/src/index.js" 2>/dev/null)" "console.log('hi')" \
  "a tracked file the run clobbered is restored from HEAD"
assert_file_exists "$REPO/src/doomed.js" \
  "a tracked file the run deleted is restored from HEAD"

assert_file_exists "$REPO/scratch-note.md" \
  "an untracked file that predates the run is left alone"
assert_eq "$(cat "$REPO/README.md" 2>/dev/null)" "# readme, edited by a human" \
  "a tracked file already dirty before the run keeps its edits (restore undoes the run's changes, not the human's)"

# The only thing standing between this repo and its pre-run git status is
# the artifact run-driver.sh is about to move out of it.
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
[ -d "$REPO2/.specify" ] \
  && fail "with nothing to keep, the scaffolding's directories are pruned entirely" \
  || pass "with nothing to keep, the scaffolding's directories are pruned entirely"

report
