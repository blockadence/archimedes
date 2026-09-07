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
# Sourced, not run: `source repo-snapshot.sh`.

# Print a snapshot of <repo-path>'s working state: one NUL-terminated
# "<kind><TAB><path>" record per interesting path, where kind is U for an
# untracked path (including ignored ones -- a scaffolder that ships its own
# .gitignore would otherwise hide its output from us) and D for a tracked
# path that already differs from HEAD.
snapshot_repo_state() { # <repo-path>
  local repo="$1"
  git -C "$repo" ls-files --others -z | while IFS= read -r -d '' p; do
    printf 'U\t%s\0' "$p"
  done
  if git -C "$repo" rev-parse --verify -q HEAD >/dev/null; then
    git -C "$repo" diff --name-only -z HEAD | while IFS= read -r -d '' p; do
      printf 'D\t%s\0' "$p"
    done
  fi
}

# Undo everything that happened to <repo-path> since <snapshot-file> was
# taken, except for the optional <keep-relpath> arguments (the driver's
# fixed_path, which has to survive for run-driver.sh to harvest).
#
# Paths that appeared since the snapshot are deleted; tracked paths that
# have gone dirty since the snapshot are restored from HEAD. Paths already
# listed in the snapshot are left exactly as they are.
restore_repo_state() { # <repo-path> <snapshot-file> [<keep-relpath> ...]
  local repo="$1" snapshot="$2"; shift 2

  local -A before=() keep=()
  local kind path record
  while IFS= read -r -d '' record; do
    kind="${record%%$'\t'*}"; path="${record#*$'\t'}"
    before["$kind:$path"]=1
  done < "$snapshot"
  for path in "$@"; do keep["$path"]=1; done

  # Delete paths that weren't there before. Collected first and deleted
  # after, so removing a scaffolder's own .gitignore mid-walk can't change
  # what the rest of the walk sees.
  local -a added=()
  while IFS= read -r -d '' path; do
    [ -n "${before["U:$path"]:-}" ] && continue
    [ -n "${keep["$path"]:-}" ] && continue
    added+=("$path")
  done < <(git -C "$repo" ls-files --others -z)
  for path in "${added[@]+"${added[@]}"}"; do
    rm -f "$repo/$path"
    prune_empty_parents "$repo" "$path"
  done

  git -C "$repo" rev-parse --verify -q HEAD >/dev/null || return 0

  while IFS= read -r -d '' path; do
    [ -n "${before["D:$path"]:-}" ] && continue
    [ -n "${keep["$path"]:-}" ] && continue
    if git -C "$repo" cat-file -e "HEAD:$path" 2>/dev/null; then
      git -C "$repo" checkout -q HEAD -- "$path"
    else
      # Staged into the index but absent from HEAD: unstage, then it's just
      # another path the run added.
      git -C "$repo" rm -q --cached --force -- "$path" >/dev/null 2>&1 || true
      rm -f "$repo/$path"
      prune_empty_parents "$repo" "$path"
    fi
  done < <(git -C "$repo" diff --name-only -z HEAD)
}

# Remove now-empty directories left behind by deleting <relpath>, walking up
# toward — but never reaching — <repo-path> itself. `rmdir` refuses to touch
# a directory that still holds anything, which is exactly the wanted
# behavior for a directory that also holds a kept path.
prune_empty_parents() { # <repo-path> <relpath>
  local repo="$1" dir
  dir="$(dirname "$2")"
  while [ "$dir" != "." ] && [ "$dir" != "/" ]; do
    rmdir "$repo/$dir" 2>/dev/null || return 0
    dir="$(dirname "$dir")"
  done
}
