#!/usr/bin/env bash
# Minimal assertion helpers shared by tests/*.sh. Not a framework — this repo
# is plain bash throughout, so tests stay plain bash too.
set -uo pipefail

TESTS_RUN=0
TESTS_FAILED=0

pass() { TESTS_RUN=$((TESTS_RUN + 1)); echo "  ok: $1"; }
fail() { TESTS_RUN=$((TESTS_RUN + 1)); TESTS_FAILED=$((TESTS_FAILED + 1)); echo "  FAIL: $1" >&2; }

assert_eq() { # <actual> <expected> <label>
  if [ "$1" = "$2" ]; then pass "$3"; else fail "$3 (expected [$2], got [$1])"; fi
}

assert_contains() { # <haystack> <needle> <label>
  case "$1" in
    *"$2"*) pass "$3" ;;
    *) fail "$3 (expected to contain [$2], got [$1])" ;;
  esac
}

assert_file_exists() { # <path> <label>
  [ -f "$1" ] && pass "$2" || fail "$2 (no file at $1)"
}

assert_file_missing() { # <path> <label>
  [ -f "$1" ] && fail "$2 (unexpectedly found $1)" || pass "$2"
}

report() { # call at end of each test file
  echo "$TESTS_RUN run, $TESTS_FAILED failed"
  [ "$TESTS_FAILED" -eq 0 ]
}
