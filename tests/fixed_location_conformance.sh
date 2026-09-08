#!/usr/bin/env bash
# The floor under every driver that declares output_mode: fixed-location.
#
# That mode's contract is a joint promise, and the half Archimedes cannot
# discharge belongs to the driver: the runner harvests the declared
# fixed_path and prunes the directories that empties, and the driver has to
# leave the target repo otherwise exactly as it found it. Both shipped
# drivers keep that promise by sourcing drivers/lib/repo-snapshot.sh --
# which is optional, invisible from the manifest, and until this file
# nothing failed when it was skipped. A third driver could declare the mode,
# pass manifest validation, harvest successfully, exit zero, and leave an
# operator's repository dirty.
#
# So: find every driver declaring the mode -- by reading the manifests, not
# from a list kept here, so one added tomorrow is covered tomorrow -- point
# each at a throwaway repo, stand a stub in for whatever CLI it runs, have
# that stub write beyond the declared fixed_path the way a real scaffolder
# or a real agent session does, and ask one question afterwards: is the repo
# as it was found?
#
# This is a floor, not a replacement. What each driver does about the
# leftovers it finds is its own business and the two shipped ones answer
# differently on purpose (spec-kit restores and succeeds, pocock restores
# and fails the run naming the files), so nothing here asserts an exit
# status. tests/pocock_driver_run.sh and tests/spec_kit_driver_run.sh remain
# where each driver's own behaviour is pinned down in detail.
#
# No network and no billed call: the stubs are what keep this in the suite
# that runs on every push, rather than in the weekly
# ARCHIMEDES_TEST_LIVE_DRIVERS bucket, where a floor is no floor at all.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

# The drivers are driven rather than dissected -- nothing here sources
# repo-snapshot.sh, which would make this the first caller of a bash 4+
# library with no bash 4+ guard in front of it. Nothing in this file needs
# bash 4 either; it skips only because under an older one every
# fixed-location driver refuses at its own guard, and there is then nothing
# to hold to anything.
#
# So the version that decides is the one the *drivers* will get -- whatever
# `#!/usr/bin/env bash` finds for them -- and not this file's own. They are
# usually the same shell, and when they are not it is this file that would
# be wrong: run under an old bash by hand, it would skip a floor the drivers
# could have cleared.
DRIVER_BASH_MAJOR="$(env bash -c 'echo "${BASH_VERSINFO[0]}"' 2>/dev/null)"
if [ "${DRIVER_BASH_MAJOR:-0}" -lt 4 ]; then
  echo "skip: fixed_location_conformance.sh (the fixed-location drivers need bash 4+ and the bash they would run under is $(env bash -c 'echo "$BASH_VERSION"' 2>/dev/null), so every one of them refuses and there is nothing to check here)"
  exit 77
fi

ROOT="$(cd "$HERE/.." && pwd)"
build_archimedes || exit 1

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

REPO="$WORK/repo"
STUB_BIN="$WORK/bin"
SESSION_LOG="$WORK/session-wrote"

# What the stub session writes besides the driver's fixed_path. Named here
# rather than inside the stub because the assertions below have to ask
# whether these ever reached the repo: a driver that failed before its
# session ran would leave the repo pristine and pass a check that never
# happened.
export CONFORMANCE_LEFTOVER="conformance-leftover.md"
export CONFORMANCE_LEFTOVER_DIR="conformance-leftovers"
export CONFORMANCE_EMPTY_DIR="conformance-empty"

# Every driver directory under <drivers-dir> whose manifest declares
# fixed-location, name-ordered. Read from the manifests because that is
# where a driver declares the mode: a list kept here would be a second
# answer to "which drivers promise this", and the whole point is that the
# next driver to make the promise is covered by making it.
fixed_location_drivers() { # <drivers-dir>
  local manifest
  for manifest in "$1"/*/driver.yaml; do
    [ -e "$manifest" ] || continue
    [ "$(manifest_field "$manifest" output_mode)" = "fixed-location" ] || continue
    basename "$(dirname "$manifest")"
  done | sort
}

# The external CLIs a driver's command runs, taken from the `command -v`
# guards it makes before it touches anybody's repo -- the one place a driver
# says out loud what it wraps. Stubbing exactly those is what lets this
# stand a session up for a driver nobody here has read: the seam is the same
# one tests/pocock_driver_run.sh and tests/spec_kit_driver_run.sh use, a
# name earlier on PATH.
#
# A driver that names none is not covered rather than quietly passed over --
# see the refusal below, which exists because the alternative is this file
# making the real, billed call it was written to avoid.
#
# All of them or none of them, and for the same reason: a guard written
# `command -v "$CLI"` names nothing that can be read off the page, and
# answering with the rest would be the worst outcome available -- the driver
# run with some of what it wraps stood in for and the remainder real.
session_clis() { # <driver-command>
  local guards names
  guards="$(grep -c '^[[:space:]]*command -v ' "$1")"
  [ "$guards" -gt 0 ] || return 0
  names="$(sed -n 's/^[[:space:]]*command -v \([A-Za-z0-9_.+-][A-Za-z0-9_.+-]*\).*/\1/p' "$1")"
  [ "$guards" -eq "$(printf '%s\n' "$names" | grep -c .)" ] || return 0

  # git and bash are never stood in for. A driver naming either is naming a
  # dependency rather than the thing it runs, and putting a stub earlier on
  # PATH under one of those names would replace what this check is made of:
  # the fixture repo, the snapshot the drivers diff against, and the stub
  # session itself all go through them. A driver whose guards name nothing
  # else is refused below, which is the safe end of that.
  printf '%s\n' "$names" | sort -u | grep -vxF -e git -e bash
}

# Stands in for whatever CLI a driver runs inside the target repo. One stub
# serves every driver because every driver in this mode is in the same
# position: something it does not control writes into someone else's
# repository, and the driver has to answer for what that something wrote.
make_session_stub() { # <bin-dir> <cli-name>
  cat > "$1/$2" <<'STUB'
#!/usr/bin/env bash
# A session that did what it was asked and then some -- `specify init`
# unpacking a toolchain, an agent handed the run of a repo and asked in a
# sentence for one file. It writes the driver's declared fixed_path, and
# then writes past it in every shape a cleanup can miss.
#
# Everything is written through $CONFORMANCE_REPO rather than into $PWD, so
# a driver that runs its session from somewhere other than the repo is
# still given a repo to answer for.
set -uo pipefail

repo="$CONFORMANCE_REPO"
fixed="$CONFORMANCE_FIXED_PATH"

# Each call writes something different at the fixed path: a driver may
# compare what its session produced against what was there before it --
# spec-kit's does, to catch a session that did nothing -- and two identical
# writes would read as exactly that.
calls=$(( $(cat "$CONFORMANCE_LOG.calls" 2>/dev/null || echo 0) + 1 ))
printf '%s\n' "$calls" > "$CONFORMANCE_LOG.calls"

# Recorded outside the repo, because if the driver is doing its job every
# one of these is gone by the time anything looks: the log is the only
# evidence left that the session ran at all.
wrote() { printf '%s\n' "$1" >> "$CONFORMANCE_LOG"; }

write_file() { # <relpath> <content>
  # Never the fixed path, whatever a driver declares it to be. That one the
  # driver is allowed to keep, so writing junk there would be asking it to
  # fail a promise it never made.
  case "$fixed" in "$1" | "$1"/*) return 0 ;; esac
  mkdir -p "$(dirname "$repo/$1")"
  printf '%s\n' "$2" > "$repo/$1"
  wrote "$1"
}

mkdir -p "$(dirname "$repo/$fixed")"
printf '# the one file the driver was run for (write %s)\n' "$calls" > "$repo/$fixed"
wrote "$fixed"

write_file "$CONFORMANCE_LEFTOVER" "an untracked file at the root of the repo"
write_file "$CONFORMANCE_LEFTOVER_DIR/notes/left-behind.md" "a file under directories the run created"
# A scaffolder shipping its own .gitignore, and something for it to hide: a
# cleanup that asked git which paths were untracked without asking for the
# ignored ones too walks straight past this.
write_file "$CONFORMANCE_LEFTOVER_DIR/.gitignore" "hidden/"
write_file "$CONFORMANCE_LEFTOVER_DIR/hidden/quiet.txt" "an ignored file"

# An edit to a file that was already tracked -- a change rather than a
# leftover, and the one a cleanup written as "delete whatever appeared"
# leaves sitting in the operator's working tree.
tracked="$(git -C "$repo" ls-files | head -1)"
if [ -n "$tracked" ] && [ "$tracked" != "$fixed" ]; then
  printf '// tidied up while I was here\n' >> "$repo/$tracked"
  wrote "$tracked"
fi

# A directory the run created and left empty. git tracks no directories, so
# a driver that asked `git status` whether it had finished cleaning up would
# be told yes.
case "$fixed" in
  "$CONFORMANCE_EMPTY_DIR" | "$CONFORMANCE_EMPTY_DIR"/*) ;;
  *)
    mkdir -p "$repo/$CONFORMANCE_EMPTY_DIR/inner"
    wrote "$CONFORMANCE_EMPTY_DIR/"
    ;;
esac

exit 0
STUB
  chmod +x "$1/$2"
}

# Point <driver-name>, resolved out of <drivers-dir>, at a fresh throwaway
# repo with a stub standing in for every CLI it runs. Leaves the repo at
# $REPO and the session's account of itself at $SESSION_LOG for the caller
# to judge -- what counts as a pass differs between the drivers this holds
# to the contract and the deliberately broken one that proves it can tell.
#
# Returns non-zero, silently, only when the check cannot be carried out at
# all -- the driver names no CLI to stand in for.
run_with_misbehaving_session() { # <drivers-dir> <driver-name>
  local dir="$1" name="$2" manifest driver_command fixed clis cli
  manifest="$dir/$name/driver.yaml"
  driver_command="$dir/$name/$(manifest_field "$manifest" command)"
  fixed="$(manifest_field "$manifest" fixed_path)"

  # Refused rather than run: with nothing stubbed the driver would reach
  # whatever it actually wraps, and for both drivers here that is a real,
  # billed, minutes-long `claude -p` inside the suite that runs on every
  # push. The caller says so; this only declines.
  clis="$(session_clis "$driver_command")"
  [ -n "$clis" ] || return 1

  rm -rf "$STUB_BIN"
  mkdir -p "$STUB_BIN"
  # Read rather than expanded, the way tests/run-all.sh reads its names: an
  # unquoted expansion would split and glob whatever a driver happened to
  # have written on that line.
  while IFS= read -r cli; do
    [ -n "$cli" ] && make_session_stub "$STUB_BIN" "$cli"
  done <<< "$clis"

  rm -rf "$REPO"
  make_widget_repo "$REPO"
  rm -f "$SESSION_LOG" "$SESSION_LOG.calls" "$WORK/harvested.md"

  # ARCHIMEDES_DRIVERS_DIR rather than the copy inside the binary, so the
  # drivers this runs are the same files the discovery above read. Which
  # layer supplies a driver is tests/driver_ownership.sh's question.
  PATH="$STUB_BIN:$PATH" \
  ARCHIMEDES_DRIVERS_DIR="$dir" \
  CONFORMANCE_REPO="$REPO" \
  CONFORMANCE_FIXED_PATH="$fixed" \
  CONFORMANCE_LOG="$SESSION_LOG" \
    "$ARCHIMEDES_BIN" run-driver "$name" "$REPO" "$WORK/harvested.md" \
    >"$WORK/run.log" 2>&1
  return 0
}

# Whether the run actually reached a session that wrote past the fixed
# path. Asked before the repo is judged, because a driver that refused at
# its first dependency check leaves a pristine repo too, and would pass a
# check that never took place.
session_wrote_beyond_the_fixed_path() {
  grep -qxF "$CONFORMANCE_LEFTOVER" "$SESSION_LOG" 2>/dev/null
}

# The verdict on a run that has just happened, for a driver expected to keep
# the contract: did the session get far enough for there to be anything to
# keep, and did the repo come back as it was found? Written once because the
# shipped drivers and the third one written to pass are asked exactly the
# same thing, and two spellings of it would be two floors.
# <driver-name> <fixed-path>
assert_kept_the_repo_as_it_found_it() {
  if session_wrote_beyond_the_fixed_path; then
    pass "$1: the run reached a session, and the session wrote into the repo beyond $2"
  else
    fail "$1: the run reached a session, and the session wrote into the repo beyond $2"
    cat "$WORK/run.log" >&2
  fi
  assert_widget_repo_pristine "$REPO" "$1"
}

echo "which drivers this covers:"

SHIPPED="$(fixed_location_drivers "$ROOT/drivers")"
if [ -n "$SHIPPED" ]; then
  pass "the drivers declaring fixed-location are found by reading the manifests: $(echo "$SHIPPED" | tr '\n' ' ')"
else
  fail "the drivers declaring fixed-location are found by reading the manifests (found none, so everything below would pass vacuously)"
fi

while IFS= read -r name; do
  [ -n "$name" ] || continue
  echo ""
  echo "$name, run against a session that writes beyond its fixed_path:"

  if ! run_with_misbehaving_session "$ROOT/drivers" "$name"; then
    fail "$name: names every CLI it runs in a \`command -v <name>\` guard, so a stub session can be stood up for it -- without one this check would have to run whatever the driver really wraps, which in this mode is a billed call"
    continue
  fi

  assert_kept_the_repo_as_it_found_it "$name" \
    "$(manifest_field "$ROOT/drivers/$name/driver.yaml" fixed_path)"
done <<< "$SHIPPED"

echo ""
echo "the check itself, against drivers written to fail it and to pass it:"

# A drivers directory of its own, holding a third driver that keeps the
# contract and a third driver that does not. Both are what this file exists
# for: neither has a test of its own anywhere, and the point is that neither
# needs one.
SCRATCH="$WORK/drivers"
mkdir -p "$SCRATCH"
cp -R "$ROOT/drivers/lib" "$SCRATCH/lib"

# Keeps the contract, and gets there the documented way: guard for bash 4,
# source the shared helpers at ../lib/, snapshot before anything runs, put
# the repo back on every exit path, keep only the declared fixed_path.
# Nothing here is copied from either shipped driver's specifics -- it is the
# route drivers/README.md describes, written out by someone reading it.
mkdir -p "$SCRATCH/conformant"
cat > "$SCRATCH/conformant/driver.yaml" <<'YAML'
name: conformant
description: A third fixed-location driver that keeps the pristine-repo contract.
output_mode: fixed-location
fixed_path: THIRD.md
command: run.sh
YAML
cat > "$SCRATCH/conformant/run.sh" <<'DRIVER'
#!/usr/bin/env bash
set -euo pipefail
[ $# -eq 1 ] || { echo "usage: run.sh <repo-path>" >&2; exit 1; }
REPO_PATH="$1"
FIXED="THIRD.md"

command -v conformance-session >/dev/null 2>&1 || {
  echo "conformance-session CLI not found on PATH" >&2; exit 1; }
[ "${BASH_VERSINFO[0]}" -ge 4 ] || {
  echo "this driver needs bash 4+ (running ${BASH_VERSION})" >&2; exit 1; }

source "$(dirname "${BASH_SOURCE[0]}")/../lib/repo-snapshot.sh"

SNAPSHOT="$(mktemp)"
snapshot_repo_state "$REPO_PATH" > "$SNAPSHOT"

RESTORE_ON_EXIT=1
cleanup() {
  local status=$?
  if [ "$RESTORE_ON_EXIT" -eq 1 ]; then
    restore_repo_state "$REPO_PATH" "$SNAPSHOT" \
      || echo "could not roll $REPO_PATH back to how it was found" >&2
  fi
  rm -f "$SNAPSHOT"
  exit "$status"
}
trap cleanup EXIT

( cd "$REPO_PATH" && conformance-session ) >&2 || {
  echo "the session failed" >&2; exit 1; }
[ -f "$REPO_PATH/$FIXED" ] || { echo "the session did not write $FIXED" >&2; exit 1; }

restore_repo_state "$REPO_PATH" "$SNAPSHOT" "$FIXED"
RESTORE_ON_EXIT=0
DRIVER

# Does not keep the contract, and does not have to try: it declares the
# mode, runs its session, and walks away. This is precisely the driver 38
# left reachable -- valid manifest, successful harvest, zero exit, dirty
# repository -- and having it here is what stops this file passing because
# it never really looked.
mkdir -p "$SCRATCH/leaky"
cat > "$SCRATCH/leaky/driver.yaml" <<'YAML'
name: leaky
description: A third fixed-location driver that never puts the repo back.
output_mode: fixed-location
fixed_path: LEAKY.md
command: run.sh
YAML
cat > "$SCRATCH/leaky/run.sh" <<'DRIVER'
#!/usr/bin/env bash
set -euo pipefail
[ $# -eq 1 ] || { echo "usage: run.sh <repo-path>" >&2; exit 1; }
command -v conformance-session >/dev/null 2>&1 || {
  echo "conformance-session CLI not found on PATH" >&2; exit 1; }
( cd "$1" && conformance-session ) >&2
DRIVER

# Declares the other mode, so it must not be picked up: the check is on the
# promise, and this driver never made it.
mkdir -p "$SCRATCH/elsewhere"
cat > "$SCRATCH/elsewhere/driver.yaml" <<'YAML'
name: elsewhere
description: A path-parameterized driver, which promises nothing about the repo.
output_mode: path-parameterized
command: run.sh
YAML
cat > "$SCRATCH/elsewhere/run.sh" <<'DRIVER'
#!/usr/bin/env bash
set -euo pipefail
echo "elsewhere saw repo $1" > "$2"
DRIVER

# Declares the mode and says nothing about what it wraps, so nothing can be
# stood in for it. The danger this guards is specific: unstubbed, both
# shipped drivers reach a real `claude -p`, and a check that quietly ran one
# would cost money on every push and take minutes doing it.
mkdir -p "$SCRATCH/undeclared"
cat > "$SCRATCH/undeclared/driver.yaml" <<'YAML'
name: undeclared
description: A fixed-location driver that never says which CLI it runs.
output_mode: fixed-location
fixed_path: UNDECLARED.md
command: run.sh
YAML
cat > "$SCRATCH/undeclared/run.sh" <<'DRIVER'
#!/usr/bin/env bash
set -euo pipefail
( cd "$1" && some-cli-nobody-declared ) >&2
DRIVER

# The same danger in its likelier shape: one guard naming a CLI plainly and
# one naming it through a variable. Reading the first and stopping there
# would leave the second unstubbed, which is the billed call arriving by the
# side door -- so this is refused as squarely as the one above.
mkdir -p "$SCRATCH/partly-declared"
cat > "$SCRATCH/partly-declared/driver.yaml" <<'YAML'
name: partly-declared
description: A fixed-location driver naming only some of the CLIs it runs.
output_mode: fixed-location
fixed_path: PARTLY.md
command: run.sh
YAML
cat > "$SCRATCH/partly-declared/run.sh" <<'DRIVER'
#!/usr/bin/env bash
set -euo pipefail
SESSION_CLI="${SESSION_CLI:-conformance-session}"
command -v conformance-session >/dev/null 2>&1 || {
  echo "conformance-session CLI not found on PATH" >&2; exit 1; }
command -v "$SESSION_CLI" >/dev/null 2>&1 || {
  echo "$SESSION_CLI not found on PATH" >&2; exit 1; }
( cd "$1" && "$SESSION_CLI" ) >&2
DRIVER

# Guards for git, which every driver in this mode depends on and none of
# them runs a session through. Standing a stub in under that name would take
# out the fixture repo, the snapshot the driver diffs against, and the stub
# session itself -- so git is never stood in for, and a driver naming
# nothing else is refused rather than sabotaged.
mkdir -p "$SCRATCH/git-guarded"
cat > "$SCRATCH/git-guarded/driver.yaml" <<'YAML'
name: git-guarded
description: A fixed-location driver whose only named dependency is git itself.
output_mode: fixed-location
fixed_path: GIT-GUARDED.md
command: run.sh
YAML
cat > "$SCRATCH/git-guarded/run.sh" <<'DRIVER'
#!/usr/bin/env bash
set -euo pipefail
command -v git >/dev/null 2>&1 || { echo "git not found on PATH" >&2; exit 1; }
git -C "$1" rev-parse --git-dir >/dev/null
echo "# a map" > "$1/GIT-GUARDED.md"
DRIVER

chmod +x "$SCRATCH"/*/run.sh

assert_eq "$(fixed_location_drivers "$SCRATCH" | tr '\n' ' ')" "conformant git-guarded leaky partly-declared undeclared " \
  "the drivers declaring fixed-location are the ones picked up -- not the one declaring another mode, and not the lib/ beside them"

# Nothing below reads $REPO or $SESSION_LOG without this having succeeded:
# a refusal returns before either is made afresh, so an unchecked call would
# quietly hand the previous driver's repo to the next driver's verdict.
run_with_misbehaving_session "$SCRATCH" "conformant" \
  || fail "conformant: names the CLI it runs, so a session can be stood up for it"
assert_kept_the_repo_as_it_found_it "conformant" "THIRD.md"
assert_file_exists "$WORK/harvested.md" \
  "a third driver written the documented way passes this check, with no test of its own to prove it"

run_with_misbehaving_session "$SCRATCH" "leaky" \
  || fail "leaky: names the CLI it runs, so a session can be stood up for it"
if session_wrote_beyond_the_fixed_path && ! widget_repo_is_pristine "$REPO"; then
  pass "a fixed-location driver that leaves files behind is caught -- which is what makes the passes above mean anything"
else
  fail "a fixed-location driver that leaves files behind is caught (the repo came back pristine, so this check cannot tell)"
  cat "$WORK/run.log" >&2
fi
assert_file_exists "$WORK/harvested.md" \
  "and it is caught having otherwise succeeded: valid manifest, harvested map, zero exit"

for refused in undeclared partly-declared git-guarded; do
  if run_with_misbehaving_session "$SCRATCH" "$refused"; then
    fail "$refused: a driver whose session cannot be stood in for whole is refused rather than run for real"
  else
    pass "$refused: a driver whose session cannot be stood in for whole is refused rather than run for real"
  fi
done

report
