#!/usr/bin/env bash
# Shared helpers, sourced by the other scripts. Not meant to be run directly.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPOS_YAML="$ROOT/repos.yaml"
WORK_DIR="$ROOT/work"
DRIVERS_DIR="${ARCHIMEDES_DRIVERS_DIR:-$ROOT/drivers}"
DOSSIER_DIR="$ROOT/repos"

require() { command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 1; }; }
require gh; require git; require yq; require jq

repo_exists() { yq -e ".repos[] | select(.name == \"$1\")" "$REPOS_YAML" >/dev/null 2>&1; }
repo_field() { yq -r ".repos[] | select(.name == \"$1\") | .$2" "$REPOS_YAML"; }
set_repo_field() { yq -i "(.repos[] | select(.name == \"$1\") | .$2) = \"$3\"" "$REPOS_YAML"; }
top_level_field() { yq -r ".$1" "$REPOS_YAML"; }  # instance-wide (not per-repo) repos.yaml field
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

# Heading and not-yet-filled-in placeholder body for a dossier's House rules
# section, shared by write_dossier_stub (which writes them) and
# house_rules_content (which must recognize the placeholder as "no rules
# recorded yet" rather than a real one — otherwise a freshly-bootstrapped,
# never-edited dossier would get its instructional boilerplate pushed/
# injected as though it were an actual mandated rule).
HOUSE_RULES_HEADING='## House rules'
HOUSE_RULES_STUB_BODY='TBD. Mandated decisions that must be respected even if unusual — the kind of
thing a new contributor (or agent) would otherwise get wrong by using good
judgment. Kept separate from "Known gotchas" below: gotchas are surprising
facts about the repo, house rules are standing directives. Edit this section
only here — `sync-house-rules.sh` pushes a durable copy into the repo
itself, and `spawn.sh` injects an ephemeral copy into every worktree
spawned for it, so this dossier is the one place changes need to be made.'

# Scaffold a new repo's dossier stub (repos/<name>.md) if one doesn't exist
# yet. Split out of bootstrap.sh so the stub's shape — notably, "House
# rules" and "Known gotchas" as two distinct sections — is unit-testable
# without bootstrap.sh's `gh repo list` network dependency.
write_dossier_stub() { # <name> <path> <base-branch>
  local name="$1" path="$2" base="$3"
  local dossier="$DOSSIER_DIR/$name.md"
  [ -f "$dossier" ] && return 0
  mkdir -p "$DOSSIER_DIR"
  cat > "$dossier" <<EOF
# $name

**Path:** $path
**Base branch:** $base
**Depends on:** TBD
**Depended on by:** TBD

## Branching
TBD, fill in during the context-mapping / dossier pass.

## Release procedure
TBD

$HOUSE_RULES_HEADING
$HOUSE_RULES_STUB_BODY

## Known gotchas
TBD
EOF
}

# Read the House rules section body out of a repo's dossier (repos/<name>.md)
# — the single source of truth both sync-house-rules.sh (durable copy
# committed into the target repo) and materialize_worktree_context (ephemeral
# copy in a spawned worktree) read from, so editing the dossier is the only
# place a house rule ever needs to change. Trims leading/trailing blank
# lines; prints nothing if the repo has no dossier, no such section, or the
# section is still the unfilled-in stub placeholder.
house_rules_content() { # <repo>
  local dossier="$DOSSIER_DIR/$1.md"
  [ -f "$dossier" ] || return 0
  local body
  body="$(awk -v heading="$HOUSE_RULES_HEADING" '
    $0 == heading { found=1; next }
    found && /^## / { exit }
    found { buf[++n] = $0 }
    END {
      start = 1; end = n
      while (start <= end && buf[start] == "") start++
      while (end >= start && buf[end] == "") end--
      for (i = start; i <= end; i++) print buf[i]
    }
  ' "$dossier")"
  if [ "$body" = "$HOUSE_RULES_STUB_BODY" ]; then
    return 0
  fi
  printf '%s' "$body"
}

# Copy this unit of work's control-repo directory (work/<slug>/, whatever
# reference material it holds — a ticket, a spec, notes) into a freshly
# spawned worktree, at the conventional $CONTEXT_DIR_NAME location, and
# guarantee it can never end up in a commit there. $STATUS_FILE_NAME is
# Archimedes' own cross-repo bookkeeping (other worktrees' local paths for
# this slug), not reference material, so it's excluded from the copy.
# Also drops in the target repo's house rules (see house_rules_content
# above) unconditionally, independent of whether this slug's work/ dir has
# anything in it — house rules apply to every worktree of the repo, not just
# ones with their own reference material.
materialize_worktree_context() { # <repo-path> <repo-name> <slug> <worktree-path>
  local repo_path="$1" repo_name="$2" slug="$3" wt="$4"
  ignore_worktree_artifacts "$repo_path"

  local src="$WORK_DIR/$slug" has_work=0
  [ -d "$src" ] && [ -n "$(find "$src" -mindepth 1 -print -quit 2>/dev/null)" ] && has_work=1

  local rules
  rules="$(house_rules_content "$repo_name")"

  [ "$has_work" -eq 1 ] || [ -n "$rules" ] || return 0

  mkdir -p "$wt/$CONTEXT_DIR_NAME"
  if [ "$has_work" -eq 1 ]; then
    cp -R "$src/." "$wt/$CONTEXT_DIR_NAME/"
    rm -f "$wt/$CONTEXT_DIR_NAME/$STATUS_FILE_NAME"
  fi
  if [ -n "$rules" ]; then
    printf '%s\n' "$rules" > "$wt/$CONTEXT_DIR_NAME/HOUSE_RULES.md"
  fi
}
