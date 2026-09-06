#!/usr/bin/env bash
# End-to-end: runs the real pocock driver, through the same run-driver.sh
# seam context-map-all.sh uses, against a throwaway git repo. Exercises the
# fixed-location contract's actual guarantees: the canonical CONTEXT.md
# lands in the control repo (here, $WORK) and the target repo is left with
# no trace of it -- clean `git status`.
#
# This makes a real, billed `claude -p` call, so it's opt-in: set
# ARCHIMEDES_TEST_LIVE_DRIVERS=1 to run it. Skips with a clear message
# otherwise, same spirit as openspec-driver-e2e.sh skipping when the
# openspec CLI isn't on PATH.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
RUN_DRIVER="$ROOT/template/scripts/run-driver.sh"
export ARCHIMEDES_DRIVERS_DIR="$ROOT/template/drivers"

if [ "${ARCHIMEDES_TEST_LIVE_DRIVERS:-0}" != "1" ]; then
  echo "skip: pocock-driver-e2e.sh (makes a real claude -p call -- set ARCHIMEDES_TEST_LIVE_DRIVERS=1 to run it)"
  exit 0
fi

if ! command -v claude >/dev/null 2>&1; then
  echo "skip: pocock-driver-e2e.sh (claude CLI not on PATH)"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
REPO="$WORK/throwaway-repo"
mkdir -p "$REPO/src"
(
  cd "$REPO"
  git init -q
  cat > src/index.js <<'EOF'
// A tiny widget-catalog service: Widgets have a name and a price.
class Widget {
  constructor(name, priceCents) {
    this.name = name;
    this.priceCents = priceCents;
  }
}
module.exports = { Widget };
EOF
  git add -A
  git -c user.email=test@example.com -c user.name=test commit -qm init
)

echo "pocock driver end-to-end:"

OUT="$WORK/CONTEXT.md"
if "$RUN_DRIVER" pocock "$REPO" "$OUT" >"$WORK/run.log" 2>&1; then
  pass "driver run exits zero against a throwaway repo"
else
  fail "driver run exits zero against a throwaway repo"
  cat "$WORK/run.log" >&2
fi

assert_file_exists "$OUT" "context map lands at the exact requested path in the control repo"

status="$(git -C "$REPO" status --porcelain)"
assert_eq "$status" "" "target repo has no trace of the artifact after harvesting (clean git status)"
assert_file_missing "$REPO/CONTEXT.md" "CONTEXT.md is gone from the target repo, not just untracked"

report
