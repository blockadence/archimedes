#!/usr/bin/env bash
# Remove worktrees/branches whose PR merged or closed. Destructive, so it
# only lists candidates unless --force is passed.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

FORCE=0; SLUG_FILTER=""
for arg in "$@"; do
  [ "$arg" = "--force" ] && FORCE=1 || SLUG_FILTER="$arg"
done

for status in "$WORK_DIR"/*/status.md; do
  [ -f "$status" ] || continue
  slug=$(basename "$(dirname "$status")")
  [ -n "$SLUG_FILTER" ] && [ "$slug" != "$SLUG_FILTER" ] && continue

  while IFS='|' read -r _ repo _ wt note _; do
    repo=$(echo "${repo:-}" | xargs); wt=$(echo "${wt:-}" | xargs)
    [ -z "$repo" ] && continue

    pr_state=$(gh pr list --repo "$(gh_slug "$repo")" --head "$slug" --json state --jq '.[0].state // "NONE"' 2>/dev/null || echo NONE)
    [ "$pr_state" = "MERGED" ] || [ "$pr_state" = "CLOSED" ] || continue

    still_needed=$(grep -rl "stacked on $repo:$slug" "$WORK_DIR"/*/status.md 2>/dev/null || true)
    if [ -n "$still_needed" ]; then
      echo "SKIP $repo:$slug ($pr_state), still a base for: $still_needed. Rebase that one first."
      continue
    fi

    echo "PRUNE CANDIDATE: $repo:$slug ($pr_state) at $wt"
    if [ "$FORCE" -eq 1 ]; then
      git -C "$(repo_path "$repo")" worktree remove "$wt" --force
      git -C "$(repo_path "$repo")" branch -D "$slug" 2>/dev/null || true
      sed -i.bak "/| $repo |/d" "$status" && rm -f "$status.bak"
      echo "  removed."
    fi
  done < <(tail -n +5 "$status")
done

[ "$FORCE" -eq 0 ] && { echo ""; echo "Dry run. Re-run with --force to actually remove the above."; }
