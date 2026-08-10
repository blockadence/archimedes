#!/usr/bin/env bash
# Shared helpers, sourced by the other scripts. Not meant to be run directly.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPOS_YAML="$ROOT/repos.yaml"
WORK_DIR="$ROOT/work"

require() { command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 1; }; }
require gh; require git; require yq; require jq

repo_field() { yq -r ".repos[] | select(.name == \"$1\") | .$2" "$REPOS_YAML"; }
repo_path()  { echo "$ROOT/$(repo_field "$1" path)"; }
worktree_path() { echo "$(repo_path "$1")-worktrees/$2"; }  # <repo> <slug>
status_file() { echo "$WORK_DIR/$1/status.md"; }             # <slug>

gh_slug() { # <repo> -> "owner/name" gh needs, read off the actual remote
  git -C "$(repo_path "$1")" remote get-url origin | sed -E 's#.*github\.com[:/](.+)\.git#\1#'
}
