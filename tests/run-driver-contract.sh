#!/usr/bin/env bash
# Exercises the generic driver-invocation contract (scripts/run-driver.sh)
# against stub drivers under tests/fixtures/drivers/, independent of any real
# driver like openspec. Run: tests/run-driver-contract.sh
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
RUN_DRIVER="$ROOT/template/scripts/run-driver.sh"
export ARCHIMEDES_DRIVERS_DIR="$HERE/fixtures/drivers"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
REPO="$WORK/repo"
mkdir -p "$REPO"

echo "run-driver.sh contract:"

out_path="$WORK/unknown.md"
if err="$("$RUN_DRIVER" nonexistent-driver "$REPO" "$out_path" 2>&1 >/dev/null)"; then
  fail "unknown driver name exits non-zero"
else
  pass "unknown driver name exits non-zero"
fi
assert_contains "$err" "nonexistent-driver" "unknown driver error names the driver"
assert_file_missing "$out_path" "unknown driver leaves no output file"

out_path="$WORK/bad-mode.md"
if err="$("$RUN_DRIVER" stub-bad-mode "$REPO" "$out_path" 2>&1 >/dev/null)"; then
  fail "unsupported output_mode exits non-zero"
else
  pass "unsupported output_mode exits non-zero"
fi
assert_contains "$err" "fixed-location" "unsupported output_mode error names the declared mode"
assert_file_missing "$out_path" "unsupported output_mode leaves no output file"

out_path="$WORK/nested/dir/ok.md"
if "$RUN_DRIVER" stub-ok "$REPO" "$out_path" >/dev/null 2>&1; then
  pass "well-behaved driver exits zero"
else
  fail "well-behaved driver exits zero"
fi
assert_file_exists "$out_path" "well-behaved driver writes to exact output path, creating parent dirs"
assert_contains "$(cat "$out_path" 2>/dev/null)" "stub-ok saw repo $REPO" "output file content came from the driver, with the resolved repo path"

out_path="$WORK/liar.md"
if err="$("$RUN_DRIVER" stub-liar "$REPO" "$out_path" 2>&1 >/dev/null)"; then
  fail "driver exiting 0 without writing output is caught as an error"
else
  pass "driver exiting 0 without writing output is caught as an error"
fi
assert_contains "$err" "$out_path" "caught error names the missing output path"

report
