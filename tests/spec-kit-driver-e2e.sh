#!/usr/bin/env bash
# End-to-end: runs the real spec-kit driver, through the same run-driver.sh
# seam context-map-all.sh uses, against a throwaway git repo. This is the
# hardest case for the fixed-location contract's guarantees -- the driver
# has to unpack a whole toolchain into the target repo to produce anything
# -- so the assertions are the same ones the pocock driver has to satisfy:
# the canonical artifact lands in the control repo (here, $WORK), and the
# target repo is left with no trace of the run at all.
#
# This makes a real, billed `claude -p` call and downloads Spec Kit's
# templates, so it's opt-in: set ARCHIMEDES_TEST_LIVE_DRIVERS=1 to run it.
# Skips with a clear message otherwise, same as pocock-driver-e2e.sh.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
RUN_DRIVER="$ROOT/template/scripts/run-driver.sh"
export ARCHIMEDES_DRIVERS_DIR="$ROOT/template/drivers"

if [ "${ARCHIMEDES_TEST_LIVE_DRIVERS:-0}" != "1" ]; then
  echo "skip: spec-kit-driver-e2e.sh (makes a real claude -p call -- set ARCHIMEDES_TEST_LIVE_DRIVERS=1 to run it)"
  exit 0
fi

if ! command -v specify >/dev/null 2>&1; then
  echo "skip: spec-kit-driver-e2e.sh (specify CLI not on PATH -- uv tool install specify-cli --from git+https://github.com/github/spec-kit.git)"
  exit 0
fi

if ! command -v claude >/dev/null 2>&1; then
  echo "skip: spec-kit-driver-e2e.sh (claude CLI not on PATH)"
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

echo "spec-kit driver end-to-end:"

OUT="$WORK/CONTEXT.md"
if "$RUN_DRIVER" spec-kit "$REPO" "$OUT" >"$WORK/run.log" 2>&1; then
  pass "driver run exits zero against a throwaway repo"
else
  fail "driver run exits zero against a throwaway repo"
  cat "$WORK/run.log" >&2
fi

assert_file_exists "$OUT" "context map lands at the exact requested path in the control repo"

content="$(cat "$OUT" 2>/dev/null)"
assert_contains "$content" "Constitution" "the harvested artifact is Spec Kit's constitution"
case "$content" in
  *"[PRINCIPLE_1_NAME]"*|*"[PROJECT_NAME]"*)
    fail "the constitution's placeholder tokens were filled in, not harvested as-scaffolded" ;;
  *)
    pass "the constitution's placeholder tokens were filled in, not harvested as-scaffolded" ;;
esac

status="$(git -C "$REPO" status --porcelain)"
assert_eq "$status" "" "target repo has no trace of the artifact after harvesting (clean git status)"
assert_file_missing "$REPO/.specify/memory/constitution.md" \
  "the constitution is gone from the target repo, not just untracked"

# git status can't see these: it doesn't track directories at all, so a
# scaffolded toolchain left behind in empty (or ignored) directories would
# pass the check above while very much still being there.
[ -d "$REPO/.specify" ] \
  && fail "Spec Kit's own .specify/ scaffolding is stripped back out of the target repo" \
  || pass "Spec Kit's own .specify/ scaffolding is stripped back out of the target repo"
[ -d "$REPO/.claude" ] \
  && fail "the speckit-* agent skills Spec Kit installed are stripped back out of the target repo" \
  || pass "the speckit-* agent skills Spec Kit installed are stripped back out of the target repo"
assert_eq "$(cd "$REPO" && ls -A | sort | tr '\n' ' ')" ".git src " \
  "the target repo holds exactly what it held before the run"

report
