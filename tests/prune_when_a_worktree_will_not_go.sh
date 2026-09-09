#!/usr/bin/env bash
# A `prune --force` run with more than one candidate, over a machine where
# git will not let one of them go: a locked worktree, which is git's refusal
# an operator actually meets. `git worktree remove --force` will not touch
# one.
#
# That once ended the run where it stood. The candidates behind the locked
# one had already been removed, the ones ahead of it had not been looked at,
# and the operator was left with `removed.` for some, silence about the
# rest, and a second run to find out which was which. What it does now is go
# through: every candidate ends the run with an outcome printed against it,
# git's own sentence stands under the entry it is about, and what comes back
# names the entries still waiting.
#
# Run through the installed binary rather than the Go API, because the half
# this is about is what an operator is told, and the last hop of that -- the
# error the CLI renders after the report -- exists nowhere else. An error
# carrying two git failures at once arrives there as one reflowed paragraph
# with git's two sentences run together, which is the shape this file exists
# to keep out.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

build_archimedes || exit 1

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

isolate_git "$WORK"

# `gh` is not asked anything this test is about: prune needs a PR state to
# call a row a candidate, and a stub that answers MERGED for every branch
# makes all three of them one without a network or a session.
mkdir -p "$WORK/bin"
cat > "$WORK/bin/gh" <<'STUB'
#!/usr/bin/env bash
echo '[{"state":"MERGED"}]'
STUB
chmod +x "$WORK/bin/gh"
export PATH="$WORK/bin:$PATH"

# An instance with three units of work, one per repo, spelled out rather
# than spawned: the fixture states the shape prune reads instead of agreeing
# with whatever wrote it.
INSTANCE="$WORK/instance"
mkdir -p "$INSTANCE"
printf 'repos:\n' > "$INSTANCE/repos.yaml"
for name in service-a service-b service-c; do
  make_origin_and_clone_at "$INSTANCE/$name-origin.git" "$INSTANCE/$name" README.md $'hi\n'
  printf '  - name: %s\n    path: ./%s\n    base_branch: main\n' "$name" "$name" >> "$INSTANCE/repos.yaml"

  git -C "$INSTANCE/$name" worktree add -q "$INSTANCE/$name-worktrees/$name-fix" -b "$name-fix"

  mkdir -p "$INSTANCE/work/$name-fix"
  {
    printf '# %s-fix\n\n' "$name-fix"
    printf '| repo | branch | worktree | note |\n|---|---|---|---|\n'
    printf '| %s | %s-fix | %s-worktrees/%s-fix | based on main |\n' "$name" "$name" "$name" "$name"
  } > "$INSTANCE/work/$name-fix/status.md"
done

# The first and the last candidate are the ones git will not let go of, so
# the run has to get past a failure to reach the one it can remove, and past
# that one to reach the second failure.
git -C "$INSTANCE/service-a" worktree lock "$INSTANCE/service-a-worktrees/service-a-fix"
git -C "$INSTANCE/service-c" worktree lock "$INSTANCE/service-c-worktrees/service-c-fix"

echo "prune --force over a worktree git will not remove:"

out="$(cd "$INSTANCE" && "$ARCHIMEDES_BIN" prune --root . --force 2>&1)"
status=$?

assert_eq "$status" "1" "a run that could not prune everything it listed fails"

# The middle candidate is the whole of the half-done run: it sits behind one
# failure and ahead of another, and it is gone.
assert_dir_missing "$INSTANCE/service-b-worktrees/service-b-fix" \
  "the candidate between the two failures is removed"
assert_contains "$out" "service-b-fix
  removed." "the candidate between the two failures is reported removed"

# Neither refused candidate is half-retired: git kept the worktree, so the
# branch and the row that name it are still there to find it by.
for name in service-a service-c; do
  assert_dir_exists "$INSTANCE/$name-worktrees/$name-fix" \
    "the locked worktree in $name survives"
  assert_contains "$(git -C "$INSTANCE/$name" branch --list "$name-fix")" "$name-fix" \
    "the branch in $name survives the worktree removal failing"
  assert_contains "$(cat "$INSTANCE/work/$name-fix/status.md")" "| $name |" \
    "the status.md row in $name-fix survives the worktree removal failing"
done

# Git's own sentence, with none of the tool's wrapping around it, under the
# entry it is about -- and one per entry, not both at the bottom.
for name in service-a service-c; do
  assert_contains "$out" "$INSTANCE/$name-worktrees/$name-fix
  not removed:
    git worktree remove $INSTANCE/$name-worktrees/$name-fix --force:" \
    "git's own words stand under $name's entry"
done
assert_contains "$out" "cannot remove a locked working tree" \
  "git's reason reaches the operator unwrapped"

# And what the run ends with names both of them, so an operator who has
# scrolled past the report still knows what is left to come back for.
assert_contains "$out" "service-a:service-a-fix" "the failed entries are named at the end"
assert_contains "$out" "service-c:service-c-fix" "both failed entries are named, not just the first"

report
