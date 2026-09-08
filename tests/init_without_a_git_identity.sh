#!/usr/bin/env bash
# The machine the tool meets first: git with no identity the operator ever
# configured. That is a fresh laptop, a container, a devcontainer and every CI
# runner -- and also the developer who has simply never run `git config
# --global user.name`, which is most of them.
#
# Those are not one machine but two, and this file walks both. Where the OS
# account carries a full name, git guesses an identity from it and will commit
# under `Somebody <login@their-hostname.local>` quite happily; where it does
# not, git refuses with "Please tell me who you are" and `archimedes init`
# once died there, which the whole of the suite passed straight over because
# the machine that ran it had an identity configured.
#
# What init does on both is the same thing: scaffold the instance and stop
# short of the commit. The files are the valuable half and they land, and the
# first commit is the operator's to author once they have an identity, because
# an instance is their own repository and an author they never chose would be
# in its history forever. On the guessing machine that means declining a
# commit git would have made -- so what init says has to explain that, or it
# reads as the tool being broken.
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
unconfigure_git_identity

echo "init on a machine where nobody configured git an identity:"

# There really is nothing configured, so a pass below is not this fixture
# failing to take effect on a developer's machine. Asked with
# user.useConfigOnly, which is the question init asks: an identity from config
# or the environment, and not one git guessed.
if git -C "$WORK" -c user.useConfigOnly=true var GIT_COMMITTER_IDENT >/dev/null 2>&1; then
  fail "the fixture leaves git no identity anybody configured"
  report
  exit 1
fi
pass "the fixture leaves git no identity anybody configured"

# Which of the two machines this run is on. Both must reach the same outcome,
# and only one of them can prove init declines a commit git would have made,
# so say which one ran rather than leaving a green suite ambiguous about it.
if git -C "$WORK" var GIT_COMMITTER_IDENT >/dev/null 2>&1; then
  pass "and an OS account git would guess one from, so this run proves the harder half"
else
  pass "and an OS account with nothing to guess from, so git would refuse the commit too"
fi

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

# And the half that needs an operator did not happen -- on this machine, where
# git would have done it unasked. `rev-list --all` rather than `HEAD`, so an
# unborn branch counts as zero instead of erroring.
assert_eq "$(git -C "$INSTANCE" rev-list --all --count)" "0" \
  "no commit is made without an identity somebody configured to make it under"
assert_eq "$(git -C "$INSTANCE" log --all --format='%an <%ae>' 2>/dev/null)" "" \
  "and no author nobody chose is written into the instance's history"

# git init did run, so what the operator is told to do next is one command
# rather than two.
assert_dir_exists "$INSTANCE/.git" "the instance is a git repository, ready for that commit"
assert_eq "$(git -C "$INSTANCE" diff --cached --name-only)" "" \
  "nothing is left staged, so the add the operator is told to run is the whole of it"

echo ""
echo "what it tells the operator:"

assert_contains "$out" "$INSTANCE" "it says where the instance is"
assert_contains "$out" "Not committed" "it says plainly that the commit did not happen"
assert_contains "$out" "guessed from your account" \
  "it says what it declined to commit under, which is the whole of why it stopped"
assert_contains "$out" "git makes that guess and commits under it" \
  "and that git would have gone ahead, so this does not read as a bug"
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

echo ""
echo "and on the machine where git will not commit either:"

# The other half of the pair, and the one CI is: an empty name, which is what
# a runner's accountless git actually has, so git dies with "empty ident name"
# rather than guessing. The outcome has to be the same one, reached for a
# different reason -- if these ever diverge, an operator's experience of the
# tool depends on which box they installed it on.
strip_git_identity

if out="$( "$INSTALLED" init gadgets "$WORK" 2>&1 )"; then
  pass "init succeeds there too"
else
  fail "init succeeds there too"
  echo "$out" >&2
fi

OTHER="$WORK/gadgets"
assert_file_exists "$OTHER/repos.yaml" "the instance lands whole"
assert_eq "$(git -C "$OTHER" rev-list --all --count)" "0" "with no commit"
assert_contains "$out" "Not committed" "and the same notice, so the contract does not vary by machine"
assert_contains "$out" "git config --global user.name" "naming the same setting to make"

report
