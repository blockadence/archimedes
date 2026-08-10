#!/usr/bin/env bash
# Live PR/branch status across every worktree this instance has spawned.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

SLUG_FILTER="${1:-}"
GUARDRAIL_MAX="${ARCHIMEDES_MAX_STREAMS:-3}"
count=0

printf '%-20s %-14s %-8s %-10s %-30s\n' "SLUG" "REPO" "PR#" "STATE" "NOTE"

for status in "$WORK_DIR"/*/status.md; do
  [ -f "$status" ] || continue
  slug=$(basename "$(dirname "$status")")
  [ -n "$SLUG_FILTER" ] && [ "$slug" != "$SLUG_FILTER" ] && continue

  while IFS='|' read -r _ repo _ _ note _; do
    repo=$(echo "${repo:-}" | xargs); note=$(echo "${note:-}" | xargs)
    [ -z "$repo" ] && continue

    pr_json=$(gh pr list --repo "$(gh_slug "$repo")" --head "$slug" --json number,state --jq '.[0] // {}' 2>/dev/null || echo '{}')
    pr_num=$(jq -r '.number // "-"' <<<"$pr_json")
    pr_state=$(jq -r '.state // "no PR"' <<<"$pr_json")

    printf '%-20s %-14s %-8s %-10s %-30s\n' "$slug" "$repo" "$pr_num" "$pr_state" "$note"
    count=$((count + 1))
  done < <(tail -n +5 "$status")   # skip title/blank/header/separator rows
done

echo ""
[ "$count" -gt "$GUARDRAIL_MAX" ] && \
  echo "Warning: $count active worktree streams open, guardrail is $GUARDRAIL_MAX. Consider closing some out."

echo ""
echo "TODO (not implemented yet): auto-detect stacked-rebase-needed. For any"
echo "'stacked on X:Y' note, check whether Y's worktree still exists; if"
echo "prune.sh already removed it (merged), this branch likely needs a"
echo "rebase onto the real base branch."
