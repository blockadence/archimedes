#!/usr/bin/env bash
# Snapshot/restore helpers for a driver that has to scaffold a toolchain
# into the target repo before it can produce anything.
#
# The fixed-location contract promises the target repo is left with no trace
# of the run apart from the declared fixed_path, which run-driver.sh then
# harvests. A driver like pocock keeps that promise by writing exactly one
# file. spec-kit can't: `specify init` unpacks templates, scripts and agent
# skills across the repo before its constitution can be filled in. So this
# takes the other route -- record what the repo looked like first, then undo
# whatever the run added or changed afterwards.
#
# "Whatever the run added or changed" is deliberately diffed rather than
# hardcoded to spec-kit's known layout: what a given version of the CLI
# unpacks is its business, and an agent session in the middle of the run can
# touch files nobody listed. Anything that predates the snapshot is left
# alone, so a repo that was already dirty stays dirty in exactly the same
# way.
#
# Requires bash 4+ for associative arrays, same as scripts/context-map-all.sh.
# Sourced, not run: `source repo-snapshot.sh`.

# Print a snapshot of <repo-path>'s working state as NUL-terminated
# "<kind><TAB><value>" records:
#
#   H  the commit HEAD points at -- everything below is described relative
#      to it, so a run that moves HEAD invalidates the whole snapshot
#   D  a directory that already existed (git tracks none, so they have to
#      be listed explicitly or a scaffolder's leftover empty directories
#      would be invisible to both this and `git status`)
#   U  an untracked path, ignored ones included -- a scaffolder that ships
#      its own .gitignore would otherwise hide its output from us
#   M  a tracked path that already differs from HEAD
snapshot_repo_state() { # <repo-path>
  local repo="$1" head
  head="$(git -C "$repo" rev-parse --verify -q HEAD || true)"
  printf 'H\t%s\0' "$head"

  list_repo_dirs "$repo" | while IFS= read -r -d '' d; do
    printf 'D\t%s\0' "$d"
  done
  git -C "$repo" ls-files --others -z | while IFS= read -r -d '' p; do
    printf 'U\t%s\0' "$p"
  done
  if [ -n "$head" ]; then
    git -C "$repo" diff --name-only -z HEAD | while IFS= read -r -d '' p; do
      printf 'M\t%s\0' "$p"
    done
  fi
}

# Undo everything that happened to <repo-path> since <snapshot-file> was
# taken, except for the optional <keep-relpath> arguments (the driver's
# fixed_path, which has to survive for run-driver.sh to harvest).
#
# Paths that appeared since the snapshot are deleted; tracked paths that
# have gone dirty since the snapshot are restored from HEAD; directories
# that appeared and are now empty are removed. Anything already listed in
# the snapshot is left exactly as it is.
#
# Returns non-zero, having changed nothing, if HEAD has moved since the
# snapshot: every judgement below is relative to that commit, so a run that
# committed, stashed or switched branches has put the repo somewhere this
# can't safely unwind. That's a case for telling the operator, not guessing.
restore_repo_state() { # <repo-path> <snapshot-file> [<keep-relpath> ...]
  local repo="$1" snapshot="$2"; shift 2

  local -A before=() keep=()
  local kind value record head_at_snapshot=""
  while IFS= read -r -d '' record; do
    kind="${record%%$'\t'*}"; value="${record#*$'\t'}"
    if [ "$kind" = "H" ]; then head_at_snapshot="$value"; else before["$kind:$value"]=1; fi
  done < "$snapshot"
  for value in "$@"; do keep["$value"]=1; done

  local head_now
  head_now="$(git -C "$repo" rev-parse --verify -q HEAD || true)"
  if [ "$head_now" != "$head_at_snapshot" ]; then
    echo "refusing to restore $repo: HEAD moved from ${head_at_snapshot:-(no commits)} to ${head_now:-(no commits)} during the run, so what the run added can no longer be told apart from what was already committed -- the repo needs looking at by hand" >&2
    return 1
  fi

  # Delete paths that weren't there before. Collected first and deleted
  # after, so removing a scaffolder's own .gitignore mid-walk can't change
  # what the rest of the walk sees.
  local -a added=()
  while IFS= read -r -d '' value; do
    [ -n "${before["U:$value"]:-}" ] && continue
    [ -n "${keep["$value"]:-}" ] && continue
    added+=("$value")
  done < <(git -C "$repo" ls-files --others -z)
  for value in "${added[@]+"${added[@]}"}"; do
    rm -f "$repo/$value"
  done

  if [ -n "$head_now" ]; then
    while IFS= read -r -d '' value; do
      [ -n "${before["M:$value"]:-}" ] && continue
      [ -n "${keep["$value"]:-}" ] && continue
      if git -C "$repo" cat-file -e "HEAD:$value" 2>/dev/null; then
        git -C "$repo" checkout -q HEAD -- "$value"
      else
        # Staged into the index but absent from HEAD: unstage, then it's
        # just another path the run added.
        git -C "$repo" rm -q --cached --force -- "$value" >/dev/null 2>&1 || true
        rm -f "$repo/$value"
      fi
    done < <(git -C "$repo" diff --name-only -z HEAD)
  fi

  # Directories the run created, now that nothing of the run's is left in
  # them. list_repo_dirs walks deepest-first, so a nested tree collapses in
  # one pass; rmdir refuses anything that still holds something, which is
  # exactly right for a directory that also holds a kept path or predates
  # the run.
  while IFS= read -r -d '' value; do
    [ -n "${before["D:$value"]:-}" ] && continue
    rmdir "$repo/$value" 2>/dev/null || true
  done < <(list_repo_dirs "$repo")
}

# Print <repo-path>'s directories, relative and NUL-terminated, deepest
# first, skipping .git and the repo root itself.
#
# `find` is pruned at .git rather than filtering its output, so a repo with
# a large object store doesn't get walked for nothing; that leaves the
# results in parent-before-child order, so they're reversed here to get the
# deepest-first order rmdir needs.
list_repo_dirs() { # <repo-path>
  local -a dirs=()
  local d i
  while IFS= read -r -d '' d; do
    dirs+=("${d#./}")
  done < <(cd "$1" && find . -path './.git' -prune -o -type d ! -name . -print0)
  for (( i = ${#dirs[@]} - 1; i >= 0; i-- )); do
    printf '%s\0' "${dirs[i]}"
  done
}
