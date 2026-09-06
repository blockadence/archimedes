#!/usr/bin/env bash
# Well-behaved path-parameterized stub, distinct from stub-ok, so tests can
# tell which of two configured drivers actually ran.
set -euo pipefail
[ $# -eq 2 ] || { echo "usage: run.sh <repo-path> <output-path>" >&2; exit 1; }
echo "stub-ok-2 saw repo $1" > "$2"
