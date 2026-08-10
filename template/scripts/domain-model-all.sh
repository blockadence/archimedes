#!/usr/bin/env bash
# Sequence + prime a domain-modeling pass across every repo in repos.yaml,
# base/dependency repos first. Orchestration only: the modeling itself is
# an interactive, human-in-the-loop session per repo, not something this
# script can do unattended.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

names=($(yq -r '.repos[].name' "$REPOS_YAML"))
declare -A done_map
order=()
remaining=("${names[@]}")

# Kahn's algorithm: repeatedly take any not-yet-ordered repo whose
# dependencies are all already ordered.
while [ "${#remaining[@]}" -gt 0 ]; do
  progressed=0
  next_remaining=()
  for name in "${remaining[@]}"; do
    deps=($(repo_field "$name" 'depends_on[]' 2>/dev/null || true))
    ready=1
    for dep in "${deps[@]}"; do
      [ -n "${done_map[$dep]:-}" ] || ready=0
    done
    if [ "$ready" -eq 1 ]; then
      order+=("$name"); done_map[$name]=1; progressed=1
    else
      next_remaining+=("$name")
    fi
  done
  remaining=("${next_remaining[@]}")
  if [ "$progressed" -eq 0 ] && [ "${#remaining[@]}" -gt 0 ]; then
    echo "Cycle or unresolved dependency among: ${remaining[*]}. Falling back to declared order." >&2
    order+=("${remaining[@]}")
    break
  fi
done

echo "Planned order: ${order[*]}"
[ "$DRY_RUN" -eq 1 ] && exit 0

for name in "${order[@]}"; do
  path="$(repo_path "$name")"
  if [ -f "$path/CONTEXT.md" ] || [ -f "$path/CONTEXT-MAP.md" ]; then
    echo "Skipping $name, already modeled."
    continue
  fi

  deps=($(repo_field "$name" 'depends_on[]' 2>/dev/null || true))
  echo ""
  echo "=== $name ==="
  echo "Path: $path"
  if [ "${#deps[@]}" -gt 0 ]; then
    echo "Depends on (already modeled, prime the session with these):"
    for dep in "${deps[@]}"; do
      echo "  - $dep: $(repo_path "$dep")/CONTEXT.md"
    done
  fi
  echo ""
  echo "Run:"
  echo "  cd $path && claude"
  echo "First message:"
  echo "  /domain-modeling — this repo's relationship to its dependencies: <fill in from WORKSPACE-MAP.md>"
  echo ""
  read -r -p "Press enter once that session is done, to move to the next repo... " _
done

echo "Domain-modeling pass complete. Re-run render-map.sh if any dependencies changed."
