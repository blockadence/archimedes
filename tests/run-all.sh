#!/usr/bin/env bash
# Runs every test file in this directory. Exits non-zero if any of them do.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

failed=0
for t in "$HERE"/*.sh; do
  [ "$(basename "$t")" = "helpers.sh" ] && continue
  [ "$(basename "$t")" = "gitfixture.sh" ] && continue
  [ "$(basename "$t")" = "run-all.sh" ] && continue
  echo "=== $(basename "$t") ==="
  "$t" || failed=1
  echo ""
done

exit "$failed"
