#!/usr/bin/env bash
# Sequence a context-mapping pass across every repo in repos.yaml,
# dependency/base repos first, and skip any repo whose context map is
# already current for its base branch's latest commit — so re-runs are
# incremental as repos are added or merged into.
#
# Orchestration only: it never assumes a specific coding agent, skill, or
# driver. By default, building the actual context map is an interactive,
# human-in-the-loop session per repo — override ARCHIMEDES_AGENT_CMD /
# ARCHIMEDES_CONTEXT_PROMPT / ARCHIMEDES_CONTEXT_FILE below for whatever
# harness/skill set you use. Set repos.yaml's top-level `driver` field (or
# ARCHIMEDES_DRIVER, checked when that's unset) to a name under drivers/
# instead, to build every repo's map unattended via that driver's contract
# (see drivers/README.md). A repo can override this instance-wide default
# for itself alone via its own `driver` field in repos.yaml. Swapping which
# driver runs, at either level, never requires changes here.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

AGENT_CMD="${ARCHIMEDES_AGENT_CMD:-claude}"
CONTEXT_FILE="${ARCHIMEDES_CONTEXT_FILE:-CONTEXT.md}"
DEFAULT_DRIVER="$(top_level_field driver)"
[ "$DEFAULT_DRIVER" = "null" ] && DEFAULT_DRIVER=""
[ -n "$DEFAULT_DRIVER" ] || DEFAULT_DRIVER="${ARCHIMEDES_DRIVER:-}"

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

for name in "${order[@]}"; do
  path="$(repo_path "$name")"
  [ -d "$path" ] || { echo "Skipping $name, not cloned yet (run bootstrap.sh)."; continue; }

  base="$(repo_field "$name" base_branch)"
  git -C "$path" fetch origin "$base" -q
  current_sha="$(git -C "$path" rev-parse "origin/$base")"
  stored_sha="$(repo_field "$name" context_modeled_sha)"
  [ "$stored_sha" = "null" ] && stored_sha=""

  if [ "$stored_sha" = "$current_sha" ] && [ -f "$path/$CONTEXT_FILE" ]; then
    echo "Up to date: $name (@ ${current_sha:0:8})"
    continue
  fi

  if [ -z "$stored_sha" ]; then
    reason="never mapped"
  elif [ ! -f "$path/$CONTEXT_FILE" ]; then
    reason="$CONTEXT_FILE missing"
  else
    reason="stale, ${stored_sha:0:8} -> ${current_sha:0:8}"
  fi

  echo ""
  echo "=== $name ($reason) ==="
  [ "$DRY_RUN" -eq 1 ] && continue

  repo_driver="$(repo_field "$name" driver)"
  [ "$repo_driver" = "null" ] && repo_driver=""
  driver="${repo_driver:-$DEFAULT_DRIVER}"

  if [ -n "$driver" ]; then
    echo "Running driver '$driver' against $path..."
    "$(dirname "${BASH_SOURCE[0]}")/run-driver.sh" "$driver" "$path" "$path/$CONTEXT_FILE"
    echo "Wrote $path/$CONTEXT_FILE"
  else
    echo "Path: $path"
    deps=($(repo_field "$name" 'depends_on[]' 2>/dev/null || true))
    prompt="Build or refresh this repo's $CONTEXT_FILE: describe its purpose, structure, and relationship to its dependencies."
    if [ "${#deps[@]}" -gt 0 ]; then
      echo "Depends on (already mapped, prime the session with these):"
      for dep in "${deps[@]}"; do
        echo "  - $dep: $(repo_path "$dep")/$CONTEXT_FILE"
      done
      prompt="$prompt Dependencies: ${deps[*]}."
    fi
    echo ""
    echo "Run:"
    echo "  cd $path && $AGENT_CMD"
    echo "First message:"
    echo "  ${ARCHIMEDES_CONTEXT_PROMPT:-$prompt}"
    echo ""
    read -r -p "Press enter once that session is done, to record $name as mapped at ${current_sha:0:8}... " _
  fi

  set_repo_field "$name" context_modeled_sha "$current_sha"
done

echo ""
if [ "$DRY_RUN" -eq 1 ]; then
  echo "Dry run: no sessions launched, no repos.yaml changes made."
else
  echo "Context-mapping pass complete."
fi
