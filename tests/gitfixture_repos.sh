#!/usr/bin/env bash
# The repo fixtures in tests/gitfixture.sh, tested on themselves -- the bash
# twin of internal/testrepo's testrepo_test.go, and here for the same reason:
# what a fixture guarantees is what every file that uses one stops saying,
# so the guarantee has to be asserted somewhere or it is asserted nowhere.
#
# The guarantee under test is the commit identity. A fixture repo commits as
# t@t with no flags at the call site, and goes on doing so on a machine
# carrying an identity of its own in the environment -- which is the half
# `git config user.name` cannot do by itself, since git reads the
# environment ahead of every config file.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "make_repo_at, the repo that stands alone:"

REPO="$WORK/plain"
mkdir -p "$REPO/src"
echo "console.log('hi')" > "$REPO/src/index.js"
make_repo_at "$REPO"

assert_eq "$(git -C "$REPO" remote)" "" \
  "no remotes: this shape is the repo a driver is pointed at and never fetches from"
assert_eq "$(git -C "$REPO" rev-parse --abbrev-ref HEAD)" "main" \
  "on main, the branch internal/testrepo's Init uses too"
assert_eq "$(git -C "$REPO" rev-list --count HEAD)" "1" \
  "one commit, so a diff against HEAD has something to be against"
assert_eq "$(git -C "$REPO" status --porcelain)" "" \
  "and nothing left uncommitted"
assert_eq "$(git -C "$REPO" ls-tree -r --name-only HEAD)" "src/index.js" \
  "what the directory already held is what got committed, and nothing else"

echo ""
echo "make_repo_at, a directory with nothing in it yet:"

EMPTY="$WORK/empty"
make_repo_at "$EMPTY"

assert_eq "$(git -C "$EMPTY" ls-tree -r --name-only HEAD)" "README.md" \
  "a directory with nothing to commit is seeded, since an empty repo has no HEAD to work from"

echo ""
echo "make_repo_at, committing on top:"

git -C "$REPO" commit -q --allow-empty -m more
assert_eq "$(git -C "$REPO" log -1 --format=%ae)" "t@t" \
  "a commit the test makes on top needs no -c user.email, and lands as the fixture's identity"

echo ""
echo "the fixed identity beats one the test file inherited:"

# git reads GIT_AUTHOR_NAME and friends ahead of every config file, so a
# fixture that only wrote `git config user.email` would quietly commit as
# whatever the environment was carrying. Exported here in the test file's own
# shell, which is where a runner's or a developer's would be too -- the
# fixture clearing them is what everything below depends on, the follow-up
# commits and the driver subprocesses a real test file spawns alike.
export GIT_AUTHOR_NAME=inherited GIT_AUTHOR_EMAIL=inherited@example.com
export GIT_COMMITTER_NAME=inherited GIT_COMMITTER_EMAIL=inherited@example.com

INHERITED="$WORK/inherited"
make_repo_at "$INHERITED"

assert_eq "$(git -C "$INHERITED" log -1 --format='%ae %ce')" "t@t t@t" \
  "make_repo_at's own commit is the fixture's identity, not the environment's"
git -C "$INHERITED" commit -q --allow-empty -m more
assert_eq "$(git -C "$INHERITED" log -1 --format='%ae %ce')" "t@t t@t" \
  "and so is one the test makes afterwards, in the shell the fixture cleared"
assert_eq "${GIT_AUTHOR_NAME-gone} ${GIT_COMMITTER_NAME-gone}" "gone gone" \
  "cleared, rather than overridden per command: what the fixture cannot reach with -c flags is the subprocesses a test spawns"

export GIT_AUTHOR_NAME=inherited GIT_AUTHOR_EMAIL=inherited@example.com
export GIT_COMMITTER_NAME=inherited GIT_COMMITTER_EMAIL=inherited@example.com

CLONE="$WORK/clone"
make_origin_and_clone_at "$WORK/clone-origin.git" "$CLONE" README.md $'hi\n'

assert_eq "$(git -C "$CLONE" log -1 --format='%ae %ce')" "t@t t@t" \
  "make_origin_and_clone_at's initial commit is the fixture's identity too"
git -C "$CLONE" commit -q --allow-empty -m more
assert_eq "$(git -C "$CLONE" log -1 --format='%ae %ce')" "t@t t@t" \
  "and so is one the test makes in the clone afterwards"

report
