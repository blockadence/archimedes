#!/usr/bin/env bash
# End-to-end: runs the real openspec driver, through the same run-driver.sh
# seam context-map-all.sh uses, against a throwaway git repo. Requires the
# `openspec` CLI on PATH (npm install -g @fission-ai/openspec); skips with a
# clear message if it isn't available rather than failing the suite.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
RUN_DRIVER="$ROOT/template/scripts/run-driver.sh"
export ARCHIMEDES_DRIVERS_DIR="$ROOT/template/drivers"

if ! command -v openspec >/dev/null 2>&1; then
  echo "skip: openspec-driver-e2e.sh (openspec CLI not on PATH — npm install -g @fission-ai/openspec)"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
REPO="$WORK/throwaway-repo"
mkdir -p "$REPO/src"
(
  cd "$REPO"
  git init -q
  echo "console.log('hi')" > src/index.js
  git add -A
  git -c user.email=test@example.com -c user.name=test commit -qm init
)

echo "openspec driver end-to-end:"

OUT="$WORK/CONTEXT.md"
if "$RUN_DRIVER" openspec "$REPO" "$OUT" >/dev/null; then
  pass "driver run exits zero against a throwaway repo"
else
  fail "driver run exits zero against a throwaway repo"
fi

assert_file_exists "$OUT" "context map lands at the exact requested path"
content="$(cat "$OUT" 2>/dev/null)"
assert_contains "$content" "Context map for throwaway-repo" "context map is stamped with the repo it was generated for"
assert_contains "$content" "OpenSpec root" "context map contains OpenSpec's own working-context report"

report
