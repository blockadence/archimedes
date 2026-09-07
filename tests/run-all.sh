#!/usr/bin/env bash
# Runs every test file in a directory -- this one by default. Exits
# non-zero if any of them do.
#
#   tests/run-all.sh          # the suite
#   tests/run-all.sh <dir>    # any directory of test files
#
# A file that cannot run reports itself skipped by exiting 77, the status
# `make check` has used for that for decades, and the summary at the end
# names every one of them. That summary is the whole point of the exit
# code: the opt-in e2e files skip on any machine without a live-driver
# opt-in -- which is every CI runner -- and a run that skipped some of the
# suite must not read the same as a run that passed all of it.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIR="${1:-$HERE}"

passed=0
failed=0
skipped=0
# Newline-delimited rather than arrays: macOS still ships bash 3.2, where an
# empty array under `set -u` is an error to expand.
skipped_names=""
failed_names=""

for t in "$DIR"/*.sh; do
  # An unmatched glob stays literal, and running it would report a missing
  # directory as a failing test.
  [ -e "$t" ] || continue
  name="$(basename "$t")"
  case "$name" in
    helpers.sh | gitfixture.sh | run-all.sh) continue ;;
  esac

  echo "=== $name ==="
  "$t"
  case "$?" in
    0) passed=$((passed + 1)) ;;
    77) skipped=$((skipped + 1)); skipped_names="$skipped_names$name"$'\n' ;;
    *) failed=$((failed + 1)); failed_names="$failed_names$name"$'\n' ;;
  esac
  echo ""
done

echo "=== summary ==="
echo "$((passed + failed + skipped)) files: $passed passed, $failed failed, $skipped skipped"
# Read rather than expand: an unquoted $names would glob a file called
# a[0-9].sh and split one with a space in it across two lines.
name_lines() { # <label> <newline-delimited names>
  [ -n "$2" ] || return 0
  while IFS= read -r n; do
    [ -n "$n" ] && printf '  %s: %s\n' "$1" "$n"
  done <<< "$2"
}
name_lines failed "$failed_names"
name_lines skipped "$skipped_names"

[ "$failed" -eq 0 ]
