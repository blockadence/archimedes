#!/usr/bin/env bash
# Snapshot/restore helpers for a driver that has to leave the repo it was
# pointed at exactly as it found it.
#
# The fixed-location contract promises the target repo is left with no trace
# of the run apart from the declared fixed_path, which run-driver.sh then
# harvests. Neither driver that declares that mode can promise it by
# construction: `specify init` unpacks templates, scripts and agent skills
# across the repo before spec-kit's constitution can be filled in, and
# pocock hands a headless agent session the run of the repo and asks it, in
# a sentence, to write one file. So both take the same route -- record what
# the repo looked like first, then afterwards find out what the run added or
# changed, and undo it.
#
# "Whatever the run added or changed" is deliberately diffed rather than
# hardcoded to a known layout: what a given version of a CLI unpacks is its
# business, and an agent session in the middle of the run can touch files
# nobody listed. Anything that predates the snapshot is left alone, so a repo
# that was already dirty stays dirty in exactly the same way.
#
# Shared by the drivers rather than owned by one of them -- reached at
# ../lib/ relative to a driver's own directory, whichever layer supplied it.
# A second copy of this would be the one that drifts, and it would drift on
# the failure path, where nobody is watching.
#
# WHAT THE ROLLBACK CANNOT COVER. A driver calls restore_repo_state from an
# exit trap, and reaches that trap on an interrupt by trapping INT and TERM
# and exiting. That covers a Ctrl-C, a killed process tree, a `timeout` and
# a cancelled CI job -- and it covers them however the run was stopped,
# because archimedes now hands the driver every signal aimed at itself
# (internal/driver/interrupt.go) rather than a terminal happening to signal
# a whole foreground group. It leaves a window, and the window is worth
# naming here rather than in either driver, because it is a property of
# undoing a run from the outside rather than of what any one run unpacks:
#
#   * SIGKILL, and a machine that loses power, cannot be trapped at all.
#     The repo is left exactly as the session left it -- scaffolding,
#     half-written map and all -- and nothing announces that.
#
#   * A signal that was ignored when the driver started stays ignored.
#     POSIX forbids a shell from trapping or restoring one, and bash sets
#     SIGINT to SIG_IGN for anything it starts asynchronously without job
#     control, a disposition inherited through forks and execs. So a driver
#     run from a background job in a script -- which is how a harness that
#     runs drivers concurrently would start one -- cannot be Ctrl-C'd. It
#     can still be TERM'd, which is why both are trapped and not just INT.
#
#   * A trapped signal is deferred while the shell waits on the session,
#     and the trap body runs once the session returns. So the rollback
#     begins after the session process is gone, never during it, and a
#     session that ignores the signal and keeps working holds the rollback
#     up for as long as it runs.
#
#   * A second signal arriving while restore_repo_state is partway through
#     stops it partway through. What has been undone stays undone; the
#     rest does not, and the repo is left between the two states.
#     Archimedes will not be the one to send it -- it forwards the first
#     signal only, and answers the rest with a line -- but anything else
#     signalling this process still can.
#
# Each of those ends with an operator's repo dirty and no message saying
# so. Closing them needs something outside the driver process -- a runner
# that keeps the snapshot and re-runs the restore, rather than a shell
# trying to clean up after its own death.
#
# Archimedes is that outside process for the delivering and the waiting: it
# passes the signal on, stays until this rollback has finished, relays what
# the driver said about the repo, and exits with the status the driver
# chose. It is not that outside process for the snapshot, which is what the
# list above would need. A driver that never got to run its trap still
# leaves a repo nobody holds a record of.
#
# Requires bash 4+ for associative arrays, same as the drivers that source
# it. Sourced, not run: `source ../lib/repo-snapshot.sh`.

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

# What the run did to <repo-path> since <snapshot-file> was taken, as
# NUL-terminated "<kind><TAB><relpath>" records, skipping the optional
# <keep-relpath> arguments (the driver's fixed_path, which the run was for):
#
#   A  a path that has appeared since the snapshot
#   C  a tracked path that has gone dirty since the snapshot -- changed,
#      deleted, or staged
#
# This is the one reading of what a run touched. Both things a driver does
# about it come through here: undoing it (restore_repo_state) and telling
# the operator about it (paths_changed_since_snapshot). Working it out twice
# would be two answers free to disagree, and the disagreement would surface
# as a driver that reports one set of files and cleans up another.
#
# Empty directories the run created are not reported: nothing was written in
# them, so there is nothing to name. restore_repo_state prunes them anyway.
#
# What this cannot see, and a driver relying on it must not claim to: a path
# that was *already* untracked or already dirty when the snapshot was taken,
# whose contents the run then overwrote. The snapshot records those paths by
# name, not by content, so the run's write to one is indistinguishable from
# the state that predated it. Recording content instead would mean hashing
# every untracked path in the repo -- `git ls-files --others` lists ignored
# ones too, deliberately, so that is the whole of node_modules on every run.
# The consequence is worth stating plainly: a session that clobbers an
# operator's uncommitted work goes unreported, and unrestored. Closing that
# needs a cheaper way to notice a write than reading the tree.
#
# Returns non-zero, printing why, if HEAD has moved since the snapshot --
# see snapshot_head_unmoved.
changed_since_snapshot() { # <repo-path> <snapshot-file> [<keep-relpath> ...]
  local repo="$1" snapshot="$2"; shift 2

  snapshot_head_unmoved "$repo" "$snapshot" || return 1

  local -A before=() keep=()
  local record value
  while IFS= read -r -d '' record; do
    before["${record%%$'\t'*}:${record#*$'\t'}"]=1
  done < "$snapshot"
  for value in "$@"; do keep["$value"]=1; done

  while IFS= read -r -d '' value; do
    [ -n "${before["U:$value"]:-}" ] && continue
    [ -n "${keep["$value"]:-}" ] && continue
    printf 'A\t%s\0' "$value"
  done < <(git -C "$repo" ls-files --others -z)

  # Only against a commit: a repo with no commits yet has nothing tracked to
  # have gone dirty. The guard above has already established HEAD is where
  # the snapshot left it, so this asks git rather than re-reading the H
  # record it just compared.
  if git -C "$repo" rev-parse --verify -q HEAD >/dev/null; then
    while IFS= read -r -d '' value; do
      [ -n "${before["M:$value"]:-}" ] && continue
      [ -n "${keep["$value"]:-}" ] && continue
      printf 'C\t%s\0' "$value"
    done < <(git -C "$repo" diff --name-only -z HEAD)
  fi
}

# The same set, one path per line, for a driver that has to say what a
# session wrote when it was asked for one file. Kinds are dropped: an
# operator being told their repo was written to does not need to know which
# side of git's tracking line each path fell on.
#
# Newlines separate them, so a path with a newline in its name would be
# reported as two. That is a report, not a plan of action -- what actually
# gets undone comes from changed_since_snapshot's NUL-terminated records --
# and a repo holding such a path has a worse problem than this line break.
paths_changed_since_snapshot() { # <repo-path> <snapshot-file> [<keep-relpath> ...]
  local records record
  records="$(mktemp)" || return 1
  changed_since_snapshot "$@" > "$records" || { rm -f "$records"; return 1; }

  while IFS= read -r -d '' record; do
    printf '%s\n' "${record#*$'\t'}"
  done < "$records"
  rm -f "$records"
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
# snapshot.
restore_repo_state() { # <repo-path> <snapshot-file> [<keep-relpath> ...]
  local repo="$1" snapshot="$2"

  # Worked out in full, into a file, before anything is touched. Two reasons,
  # and the second is the one that bites: nothing is undone until the whole
  # list is in hand, so removing a scaffolder's own .gitignore mid-walk can't
  # change what the rest of the walk sees -- and a failure in there (the
  # moved-HEAD refusal, an unreadable snapshot, git falling over) is a status
  # this can return. Read through a process substitution instead, it would be
  # a status bash throws away, and this would report a repo put back that it
  # had not touched.
  local records
  records="$(mktemp)" || return 1
  changed_since_snapshot "$@" > "$records" || { rm -f "$records"; return 1; }

  local -a added=() changed=()
  local kind value record
  while IFS= read -r -d '' record; do
    kind="${record%%$'\t'*}"; value="${record#*$'\t'}"
    case "$kind" in
      A) added+=("$value") ;;
      C) changed+=("$value") ;;
    esac
  done < "$records"
  rm -f "$records"

  for value in "${added[@]+"${added[@]}"}"; do
    rm -f "$repo/$value"
  done

  for value in "${changed[@]+"${changed[@]}"}"; do
    if git -C "$repo" cat-file -e "HEAD:$value" 2>/dev/null; then
      git -C "$repo" checkout -q HEAD -- "$value"
    else
      # Staged into the index but absent from HEAD: unstage, then it's
      # just another path the run added.
      git -C "$repo" rm -q --cached --force -- "$value" >/dev/null 2>&1 || true
      rm -f "$repo/$value"
    fi
  done

  # Directories the run created, now that nothing of the run's is left in
  # them. Read from the snapshot rather than from changed_since_snapshot,
  # which reports no directories: git tracks none, so an empty one a run
  # leaves behind is a trace nothing else can see.
  #
  # list_repo_dirs walks deepest-first, so a nested tree collapses in one
  # pass; rmdir refuses anything that still holds something, which is
  # exactly right for a directory that also holds a kept path or predates
  # the run.
  local -A dirs_before=()
  while IFS= read -r -d '' record; do
    kind="${record%%$'\t'*}"; value="${record#*$'\t'}"
    [ "$kind" = "D" ] && dirs_before["$value"]=1
  done < "$snapshot"
  while IFS= read -r -d '' value; do
    [ -n "${dirs_before["$value"]:-}" ] && continue
    rmdir "$repo/$value" 2>/dev/null || true
  done < <(list_repo_dirs "$repo")
}

# Arm INT and TERM so that an interrupt takes the caller into its EXIT trap,
# where the rollback above lives. Call it once, after that EXIT trap is set.
#
# A driver's rollback hangs off EXIT, and an interrupt is not reliably
# something that makes a shell exit. The reasoning this replaces was that a
# Ctrl-C signals the whole process group, so the session dies with it and
# the driver's `|| abort` carries the run into the exit trap. That holds
# only when the session is *killed by* the signal. Bash defers a signal
# that arrives while it is waiting on a foreground child, and when the
# child is reaped it decides what to do with the deferred signal from how
# that child ended: died from it, and the shell re-raises it on itself;
# ended any other way, and the shell reads that as the child having handled
# the interrupt, and drops its own copy. So a session that traps SIGINT and
# shuts down cleanly, which is what a well-behaved CLI does, exits zero --
# and the run walks straight past the operator's Ctrl-C, keeps its output,
# disarms the rollback and exits zero with everything the run unpacked
# still sitting in their repo. Same for a signal that reached the driver
# alone and never touched the session.
#
# Trapping them costs the shell nothing it was relying on: a trapped signal
# is still deferred until the foreground session returns, which is the
# order the rollback has to happen in anyway. What changes is that whether
# an interrupt stops the run is no longer the session's to decide.
#
# Which signals arrive at all is likewise no longer a terminal's to decide.
# Archimedes runs a driver in a process group of its own and signals that
# group itself, so these traps fire for a `kill` by pid, a supervisor or a
# `timeout` exactly as they do for a Ctrl-C -- and the driver is handed one
# copy of the interrupt rather than one per way it could have reached the
# group.
#
# Here rather than in each driver for the reason this whole file is here:
# it is failure-path code, and a second copy of it would be the one that
# drifts. What it still does not cover is named at the top of this file.
exit_on_interrupt() { # <repo-path>
  INTERRUPTED_REPO="$1"
  trap 'interrupted_by INT 130' INT
  trap 'interrupted_by TERM 143' TERM
}

# The body those two traps run. Separate from exit_on_interrupt, and
# reaching the repo path through a variable rather than an argument,
# because a trap's command is a string evaluated when the signal arrives --
# long after the arguments it was set up with have gone.
interrupted_by() { # <signal-name> <exit-status>
  echo "interrupted by SIG$1 -- rolling $INTERRUPTED_REPO back to how it was found and keeping nothing" >&2
  exit "$2"
}

# Refuse, printing why, if <repo-path>'s HEAD is not the commit
# <snapshot-file> was taken against.
#
# Every judgement the helpers above make is relative to that commit, so a
# run that committed, stashed or switched branches has put the repo
# somewhere neither undoing nor describing the run can reach: what it added
# is now indistinguishable from what was already committed, while `git
# status` reads clean. That's a case for telling the operator, not guessing.
snapshot_head_unmoved() { # <repo-path> <snapshot-file>
  local repo="$1" snapshot="$2" record head_at_snapshot="" head_now

  while IFS= read -r -d '' record; do
    if [ "${record%%$'\t'*}" = "H" ]; then head_at_snapshot="${record#*$'\t'}"; break; fi
  done < "$snapshot"

  head_now="$(git -C "$repo" rev-parse --verify -q HEAD || true)"
  [ "$head_now" = "$head_at_snapshot" ] && return 0

  echo "refusing to touch $repo: HEAD moved from ${head_at_snapshot:-(no commits)} to ${head_now:-(no commits)} during the run, so what the run added can no longer be told apart from what was already committed -- the repo needs looking at by hand" >&2
  return 1
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
