#!/usr/bin/env bash
# The machine beside the one init_without_a_git_identity.sh walks: an
# identity configured and correct, and git refusing the first commit anyway.
# The common cause is commit signing configured with no key that works here
# -- a dotfile copied to a new laptop, a container that never had a keyring,
# a `gpg.program` pointing at something that is not installed. `commit.gpgsign
# = true` comes down from a global config, the identity is there, so nothing
# is skipped: git is asked, and dies.
#
# That once ended the run with git's wrapped-up stderr as the first thing the
# tool said and no instance to show for it. What it ends with now is the
# instance, a sentence of the tool's own, and git's reason quoted underneath
# so the operator knows which of the many possible causes theirs is.
#
# Run through the installed binary rather than the Go API, because the half
# that matters most here is what an operator is told, and that is the CLI's.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

build_archimedes || exit 1

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

INSTALLED="$WORK/bin/archimedes"
mkdir -p "$WORK/bin"
cp "$ARCHIMEDES_BIN" "$INSTALLED"

# An identity, and a signing configuration that cannot work: the machine this
# file is about. Layered onto isolate_git's config, so the identity is the one
# it wrote rather than the one this box happens to carry.
isolate_git "$WORK"
git config --global commit.gpgsign true
git config --global gpg.program /nonexistent/gpg

echo "init where git will not make the commit it is asked for:"

# The fixture is that machine and not the other one. An identity is
# configured -- asked the way init asks, under user.useConfigOnly -- so
# nothing here is the no-identity path by another name.
if git -C "$WORK" -c user.useConfigOnly=true var GIT_COMMITTER_IDENT >/dev/null 2>&1; then
  pass "the fixture leaves an identity configured, so nothing is skipped for want of one"
else
  fail "the fixture leaves an identity configured, so nothing is skipped for want of one"
  report
  exit 1
fi

# And git really does refuse to commit under it, in a scratch repo of its
# own, so a pass below is the tool's doing rather than the fixture failing to
# take effect.
PROBE="$WORK/probe"
git init -q "$PROBE"
if git -C "$PROBE" commit -q --allow-empty -m probe >/dev/null 2>&1; then
  fail "the fixture makes git refuse a commit"
  report
  exit 1
fi
pass "the fixture makes git refuse a commit"

if out="$( "$INSTALLED" init widgets "$WORK" 2>&1 )"; then
  pass "init succeeds rather than dying with git's failure"
else
  fail "init succeeds rather than dying with git's failure"
  echo "$out" >&2
  report
  exit 1
fi

INSTANCE="$WORK/widgets"

# The valuable half landed and stayed. This is the whole of what the issue
# was about: a first run that produced nothing.
for path in repos.yaml WORKSPACE-MAP.md AGENTS.md README.md; do
  assert_file_exists "$INSTANCE/$path" "the instance still carries its $path"
done
for path in repos work drivers scaffolding convention-packs; do
  assert_dir_exists "$INSTANCE/$path" "the instance still carries its $path/"
done
assert_dir_exists "$INSTANCE/.git" "the instance is a git repository, ready for that commit"

# And the commit did not happen some other way. `--no-gpg-sign` past a broken
# key would put a commit in the operator's history that contradicts what they
# configured, which is the same objection as an author nobody chose.
assert_eq "$(git -C "$INSTANCE" rev-list --all --count)" "0" \
  "no commit is made around the configuration that refused it"

echo ""
echo "what it tells the operator:"

assert_contains "$out" "$INSTANCE" "it says where the instance is"
assert_contains "$out" "Not committed" "it says plainly that the commit did not happen"
assert_contains "$out" "/nonexistent/gpg" \
  "it quotes git's own reason, which is the only thing that names what to fix"
assert_contains "$out" "not retried with your configuration turned off" \
  "and says it did not commit around that reason"
assert_not_contains "$out" "exit status" \
  "it does not hand back raw git output as the first thing it says"
assert_not_contains "$out" "git config --global user.name" \
  "it does not send an operator whose identity is fine to configure one"
assert_contains "$out" "bootstrap" "it still points at the next step"

# The assertion the rest of this file exists for: the retry it printed is one
# an operator can run, once their machine is fixed. Everything below is typed
# out of what init printed.
COMMIT_CMD="$(printf '%s\n' "$out" | sed -n 's/^ *cd .*&& \(git add -A && git commit .*\)$/\1/p')"
# Non-empty and one line, tested in that order: `wc -l` on nothing still says
# one, so a line count alone would pass on a sed that matched nothing at all.
if [ -n "$COMMIT_CMD" ] && [ "$(printf '%s\n' "$COMMIT_CMD" | wc -l | tr -d ' ')" = "1" ]; then
  pass "the commit is given as one command that can be run as printed"
else
  fail "the commit is given as one command that can be run as printed (got [$COMMIT_CMD])"
fi

git config --global commit.gpgsign false

if ( cd "$INSTANCE" && eval "$COMMIT_CMD" ) >"$WORK/commit.log" 2>&1; then
  pass "and it makes the instance's first commit once git can sign again"
else
  fail "and it makes the instance's first commit once git can sign again"
  cat "$WORK/commit.log" >&2
fi

assert_eq "$(git -C "$INSTANCE" rev-list --count HEAD)" "1" \
  "the instance ends up on the same fresh history it would have had all along"
assert_eq "$(git -C "$INSTANCE" status --porcelain)" "" \
  "with everything the template carried in that commit, dotfiles included"
assert_contains "$(git -C "$INSTANCE" log -1 --format=%s)" "widgets" \
  "under the same subject init would have used"

report
