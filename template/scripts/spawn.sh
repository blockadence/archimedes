#!/usr/bin/env bash
# Create a branch+worktree for one unit of work in one target repo.
# Always fetches first, so new branches start from current remote state,
# never from a possibly-stale local checkout.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

[ $# -ge 2 ] || { echo "usage: spawn.sh <slug> <repo> [--base <branch>|--stack-on <repo>:<slug>]"; exit 1; }
SLUG="$1"; REPO="$2"; shift 2

BASE_OVERRIDE=""; STACK_REPO=""; STACK_SLUG=""
while [ $# -gt 0 ]; do
  case "$1" in
    --base) BASE_OVERRIDE="$2"; shift 2 ;;
    --stack-on) IFS=':' read -r STACK_REPO STACK_SLUG <<<"$2"; shift 2 ;;
    *) echo "unknown arg: $1"; exit 1 ;;
  esac
done

REPO_PATH="$(repo_path "$REPO")"
[ -d "$REPO_PATH" ] || { echo "unknown repo: $REPO (run bootstrap.sh first)"; exit 1; }
BASE_BRANCH="$(repo_field "$REPO" base_branch)"

git -C "$REPO_PATH" fetch origin
git -C "$REPO_PATH" checkout "$BASE_BRANCH"
git -C "$REPO_PATH" pull --ff-only origin "$BASE_BRANCH"

if [ -n "$STACK_REPO" ]; then
  START_POINT="$STACK_SLUG"; NOTE="stacked on $STACK_REPO:$STACK_SLUG"
elif [ -n "$BASE_OVERRIDE" ]; then
  START_POINT="$BASE_OVERRIDE"; NOTE="based on $BASE_OVERRIDE"
else
  START_POINT="origin/$BASE_BRANCH"; NOTE="based on $BASE_BRANCH"
fi

WT="$(worktree_path "$REPO" "$SLUG")"
mkdir -p "$(dirname "$WT")"
git -C "$REPO_PATH" worktree add "$WT" -b "$SLUG" "$START_POINT"

mkdir -p "$WORK_DIR/$SLUG"
STATUS="$(status_file "$SLUG")"
[ -f "$STATUS" ] || printf '# %s\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n' "$SLUG" > "$STATUS"
printf '| %s | %s | %s | %s | - |\n' "$REPO" "$SLUG" "$WT" "$NOTE" >> "$STATUS"

echo "Worktree ready: $WT ($NOTE)"
echo "cd $WT && claude"
