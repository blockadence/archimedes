#!/usr/bin/env bash
# Well-behaved fixed-location stub: writes a fixed line naming its
# repo-path argument to OUT.md at that repo's root (its declared
# fixed_path) -- it is never told an output path.
set -euo pipefail
[ $# -eq 1 ] || { echo "usage: run.sh <repo-path>" >&2; exit 1; }
echo "stub-fixed-ok saw repo $1" > "$1/OUT.md"
