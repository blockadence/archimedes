#!/usr/bin/env bash
# Fixed-location stub that can only write deep inside a directory tree it
# has to create -- the shape of a driver wrapping a tool that scaffolds
# itself into the repo before it can produce anything.
set -euo pipefail
[ $# -eq 1 ] || { echo "usage: run.sh <repo-path>" >&2; exit 1; }
mkdir -p "$1/.stub/memory"
echo "stub-fixed-nested saw repo $1" > "$1/.stub/memory/OUT.md"
