#!/usr/bin/env bash
# The machine the tool meets first, and the one no developer has: git with no
# identity configured. A fresh laptop, a container, a devcontainer, and every
# CI runner. `archimedes init` runs `git init`, `git add -A` and `git commit`,
# so before this it died there with git's own "Please tell me who you are" --
# which the whole of the suite passed straight over, because the machine that
# ran it had an identity.
#
# What it does instead is scaffold the instance and stop short of the commit:
# the files are the valuable half and they land, and the first commit is the
# operator's to author once they have an identity, because an instance is
# their own repository and a made-up author would be in its history forever.
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

# Everything below this line runs as a machine that has never configured git.
strip_git_identity

echo "init on a machine with no git identity:"

# The identity really is gone, so a pass below is not this fixture failing to
# take effect on a developer's machine.
if git -C "$WORK" var GIT_COMMITTER_IDENT >/dev/null 2>&1; then
  fail "the fixture leaves git with no identity to commit under"
  report
  exit 1
fi
pass "the fixture leaves git with no identity to commit under"

if out="$( "$INSTALLED" init widgets "$WORK" 2>&1 )"; then
  pass "init succeeds rather than handing back raw git output"
else
  fail "init succeeds rather than handing back raw git output"
  echo "$out" >&2
  report
  exit 1
fi

INSTANCE="$WORK/widgets"

# The valuable half landed. Whatever init said, it said it about a real
# instance.
for path in repos.yaml WORKSPACE-MAP.md AGENTS.md README.md; do
  assert_file_exists "$INSTANCE/$path" "the instance still carries its $path"
done
for path in repos work drivers scaffolding convention-packs; do
  assert_dir_exists "$INSTANCE/$path" "the instance still carries its $path/"
done

# And the half that needs an operator did not happen. `rev-list --all` rather
# than `HEAD`, so an unborn branch counts as zero instead of erroring.
assert_eq "$(git -C "$INSTANCE" rev-list --all --count)" "0" \
  "no commit is made without an identity to make it under"
assert_eq "$(git -C "$INSTANCE" log --all --format='%an <%ae>' 2>/dev/null)" "" \
  "and no author nobody configured is written into the instance's history"

# git init did run, so what the operator is told to do next is one command
# rather than two.
assert_dir_exists "$INSTANCE/.git" "the instance is a git repository, ready for that commit"
assert_eq "$(git -C "$INSTANCE" diff --cached --name-only)" "" \
  "nothing is left staged, so the add the operator is told to run is the whole of it"

echo ""
echo "what it tells the operator:"

assert_contains "$out" "$INSTANCE" "it says where the instance is"
assert_contains "$out" "Not committed" "it says plainly that the commit did not happen"
assert_contains "$out" "git config --global user.name" "it names the setting to make"
assert_contains "$out" "git config --global user.email" "it names the other one"
assert_contains "$out" "bootstrap" "it still points at the next step"

# The assertion the rest of this file exists for: the advice is not merely
# present, it works. Everything below is typed out of what init printed.
IDENT_CMDS="$(printf '%s\n' "$out" | sed -n 's/^ *\(git config --global .*\)$/\1/p')"
COMMIT_CMD="$(printf '%s\n' "$out" | sed -n 's/^ *cd .*&& \(git add -A && git commit .*\)$/\1/p')"
assert_eq "$(printf '%s\n' "$IDENT_CMDS" | wc -l | tr -d ' ')" "2" \
  "the two settings are given as commands that can be run as printed"

# Run them where the operator would: --global, against a config of this
# test's own rather than the machine's.
export GIT_CONFIG_GLOBAL="$WORK/gitconfig"
: > "$GIT_CONFIG_GLOBAL"
unset GIT_AUTHOR_NAME GIT_COMMITTER_NAME
while IFS= read -r cmd; do
  eval "$cmd" || { fail "init's own identity commands run"; break; }
done <<< "$IDENT_CMDS"

if ( cd "$INSTANCE" && eval "$COMMIT_CMD" ) >"$WORK/commit.log" 2>&1; then
  pass "and the commit it printed then makes the instance's first commit"
else
  fail "and the commit it printed then makes the instance's first commit"
  cat "$WORK/commit.log" >&2
fi

assert_eq "$(git -C "$INSTANCE" rev-list --count HEAD)" "1" \
  "the instance ends up on the same fresh history it would have had all along"
assert_eq "$(git -C "$INSTANCE" status --porcelain)" "" \
  "with everything the template carried in that commit, dotfiles included"
assert_contains "$(git -C "$INSTANCE" log -1 --format=%s)" "widgets" \
  "under the same subject init would have used"
assert_contains "$(git -C "$INSTANCE" log -1 --format='%an <%ae>')" "Your Name" \
  "and under the operator's own identity, which is the only one it was ever going to be"

report
