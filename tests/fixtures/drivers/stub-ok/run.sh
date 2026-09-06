#!/usr/bin/env bash
# Well-behaved path-parameterized stub: writes a fixed line naming its
# repo-path argument to exactly the given output-path.
set -euo pipefail
[ $# -eq 2 ] || { echo "usage: run.sh <repo-path> <output-path>" >&2; exit 1; }
echo "stub-ok saw repo $1" > "$2"
