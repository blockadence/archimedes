#!/usr/bin/env bash
# End-to-end: repos.yaml's top-level `driver` field sets an instance-wide
# default, a single repo's own `driver` field overrides it just for that
# repo, and naming an unknown driver at either level fails clearly instead
# of silently falling back to the interactive session.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

echo "context-map-all.sh instance-wide default + per-repo override:"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

make_origin_and_clone "$WORK" default-repo
make_origin_and_clone "$WORK" override-repo
INSTANCE="$(new_test_instance "$WORK")"

cat > "$INSTANCE/repos.yaml" <<EOF
driver: stub-ok
repos:
  - name: default-repo
    path: ../default-repo
    base_branch: main
    depends_on: []
    context_modeled_sha: null
  - name: override-repo
    path: ../override-repo
    base_branch: main
    depends_on: []
    context_modeled_sha: null
    driver: stub-ok-2
EOF

(
  cd "$INSTANCE"
  export ARCHIMEDES_DRIVERS_DIR="$HERE/fixtures/drivers"
  ./scripts/context-map-all.sh
) </dev/null >"$WORK/run.log" 2>&1
run_status=$?

if [ "$run_status" -eq 0 ]; then
  pass "context-map-all.sh runs unattended using repos.yaml's top-level driver default"
else
  fail "context-map-all.sh runs unattended using repos.yaml's top-level driver default"
  cat "$WORK/run.log" >&2
fi

assert_contains "$(cat "$WORK/default-repo/CONTEXT.md" 2>/dev/null)" "stub-ok saw repo" \
  "repo with no driver override uses the instance-wide default"
assert_contains "$(cat "$WORK/override-repo/CONTEXT.md" 2>/dev/null)" "stub-ok-2 saw repo" \
  "repo's own driver field overrides the instance-wide default"

echo ""
echo "context-map-all.sh unknown driver fails clearly (instance-level):"

WORK2="$(mktemp -d)"
trap 'rm -rf "$WORK" "$WORK2"' EXIT

make_origin_and_clone "$WORK2" bad-repo
INSTANCE2="$(new_test_instance "$WORK2")"

cat > "$INSTANCE2/repos.yaml" <<EOF
driver: does-not-exist
repos:
  - name: bad-repo
    path: ../bad-repo
    base_branch: main
    depends_on: []
    context_modeled_sha: null
EOF

(
  cd "$INSTANCE2"
  export ARCHIMEDES_DRIVERS_DIR="$HERE/fixtures/drivers"
  ./scripts/context-map-all.sh
) </dev/null >"$WORK2/run.log" 2>&1
bad_status=$?

if [ "$bad_status" -ne 0 ]; then
  pass "context-map-all.sh exits non-zero for an unknown instance-level driver"
else
  fail "context-map-all.sh exits non-zero for an unknown instance-level driver"
fi

assert_contains "$(cat "$WORK2/run.log")" "unknown driver: does-not-exist" \
  "instance-level failure names the unknown driver rather than silently falling back"
assert_file_missing "$WORK2/bad-repo/CONTEXT.md" \
  "no context file is written when the instance-level driver is unknown"

echo ""
echo "context-map-all.sh unknown driver fails clearly (per-repo override):"

WORK3="$(mktemp -d)"
trap 'rm -rf "$WORK" "$WORK2" "$WORK3"' EXIT

make_origin_and_clone "$WORK3" bad-override-repo
INSTANCE3="$(new_test_instance "$WORK3")"

cat > "$INSTANCE3/repos.yaml" <<EOF
driver: stub-ok
repos:
  - name: bad-override-repo
    path: ../bad-override-repo
    base_branch: main
    depends_on: []
    context_modeled_sha: null
    driver: also-does-not-exist
EOF

(
  cd "$INSTANCE3"
  export ARCHIMEDES_DRIVERS_DIR="$HERE/fixtures/drivers"
  ./scripts/context-map-all.sh
) </dev/null >"$WORK3/run.log" 2>&1
bad_override_status=$?

if [ "$bad_override_status" -ne 0 ]; then
  pass "context-map-all.sh exits non-zero for an unknown per-repo driver override, even with a valid instance default"
else
  fail "context-map-all.sh exits non-zero for an unknown per-repo driver override, even with a valid instance default"
fi

assert_contains "$(cat "$WORK3/run.log")" "unknown driver: also-does-not-exist" \
  "per-repo override failure names the unknown driver rather than silently falling back to the instance default"
assert_file_missing "$WORK3/bad-override-repo/CONTEXT.md" \
  "no context file is written when the per-repo driver override is unknown"

report
