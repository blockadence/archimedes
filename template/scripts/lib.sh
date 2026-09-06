#!/usr/bin/env bash
# Shared helpers, sourced by the other scripts. Not meant to be run directly.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPOS_YAML="$ROOT/repos.yaml"
WORK_DIR="$ROOT/work"
DRIVERS_DIR="${ARCHIMEDES_DRIVERS_DIR:-$ROOT/drivers}"

require() { command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 1; }; }
require gh; require git; require yq; require jq

repo_exists() { yq -e ".repos[] | select(.name == \"$1\")" "$REPOS_YAML" >/dev/null 2>&1; }
repo_field() { yq -r ".repos[] | select(.name == \"$1\") | .$2" "$REPOS_YAML"; }
set_repo_field() { yq -i "(.repos[] | select(.name == \"$1\") | .$2) = \"$3\"" "$REPOS_YAML"; }
repo_path()  { echo "$ROOT/$(repo_field "$1" path)"; }
worktree_path() { echo "$(repo_path "$1")-worktrees/$2"; }  # <repo> <slug>
STATUS_FILE_NAME="status.md"
status_file() { echo "$WORK_DIR/$1/$STATUS_FILE_NAME"; }     # <slug>

gh_slug() { # <repo> -> "owner/name" gh needs, read off the actual remote
  git -C "$(repo_path "$1")" remote get-url origin | sed -E 's#.*github\.com[:/](.+)\.git#\1#'
}

# Conventional location, inside a worktree, for control-repo-owned reference
# material (specs, tickets, notes) that spawn.sh copies in. Never committed
# to the target repo — see ignore_worktree_artifacts below.
CONTEXT_DIR_NAME=".archimedes"

# Make $CONTEXT_DIR_NAME invisible to `git status`/`git add -A` in every
# worktree of <repo-path>, without touching that repo's own tracked
# .gitignore. Writes to the repo's shared (commondir) info/exclude, which is
# local-only, untracked, and honored by every worktree sharing that repo —
# so it needs setting once per repo, and there's nothing to clean up when a
# worktree is later pruned. Check-then-append, so it assumes spawn.sh isn't
# run concurrently for two slugs against the same repo (true of expected
# single-operator usage).
ignore_worktree_artifacts() { # <repo-path>
  local repo_path="$1"
  local common_dir
  common_dir="$(git -C "$repo_path" rev-parse --git-common-dir)"
  case "$common_dir" in
    /*) ;;
    *) common_dir="$repo_path/$common_dir" ;;
  esac
  local exclude_file="$common_dir/info/exclude"
  mkdir -p "$(dirname "$exclude_file")"
  grep -qxF "/$CONTEXT_DIR_NAME/" "$exclude_file" 2>/dev/null \
    || printf '/%s/\n' "$CONTEXT_DIR_NAME" >> "$exclude_file"
}

# Copy this unit of work's control-repo directory (work/<slug>/, whatever
# reference material it holds — a ticket, a spec, notes) into a freshly
# spawned worktree, at the conventional $CONTEXT_DIR_NAME location, and
# guarantee it can never end up in a commit there. $STATUS_FILE_NAME is
# Archimedes' own cross-repo bookkeeping (other worktrees' local paths for
# this slug), not reference material, so it's excluded from the copy.
materialize_worktree_context() { # <repo-path> <slug> <worktree-path>
  local repo_path="$1" slug="$2" wt="$3"
  ignore_worktree_artifacts "$repo_path"

  local src="$WORK_DIR/$slug"
  if [ ! -d "$src" ] || [ -z "$(find "$src" -mindepth 1 -print -quit 2>/dev/null)" ]; then
    return 0
  fi
  mkdir -p "$wt/$CONTEXT_DIR_NAME"
  cp -R "$src/." "$wt/$CONTEXT_DIR_NAME/"
  rm -f "$wt/$CONTEXT_DIR_NAME/$STATUS_FILE_NAME"
}
