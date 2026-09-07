#!/usr/bin/env bash
# Switching an instance to the spec-kit driver has to be a configuration
# change and nothing else -- no orchestration edits, no per-driver special
# cases anywhere. These tests prove that without running spec-kit: they set
# up a PATH the `specify` CLI isn't on, so a run gets as far as the driver's
# own dependency check and stops there. Reaching that error at all means
# repos.yaml named the driver, run-driver.sh resolved its manifest, and the
# driver's command ran -- the whole chain minus the billed part.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
RUN_DRIVER="$ROOT/template/scripts/run-driver.sh"
DRIVER_DIR="$ROOT/template/drivers/spec-kit"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# A PATH with the scripts' own dependencies on it but neither `specify` nor
# `claude`, wherever this machine happens to keep them.
STUB_BIN="$WORK/bin"
mkdir -p "$STUB_BIN"
# bash included deliberately: the scripts' `#!/usr/bin/env bash` would
# otherwise resolve to whatever /usr/bin holds, which on macOS is bash 3.2.
for tool in bash git yq jq gh; do
  resolved="$(command -v "$tool" 2>/dev/null)" && ln -sf "$resolved" "$STUB_BIN/$tool"
done
SPECLESS_PATH="$STUB_BIN:/usr/bin:/bin:/usr/sbin:/sbin"

echo "spec-kit driver manifest:"

MANIFEST="$DRIVER_DIR/driver.yaml"
assert_file_exists "$MANIFEST" "the driver ships a manifest under drivers/spec-kit/"
assert_eq "$(yq -r '.name' "$MANIFEST")" "spec-kit" "manifest name matches its directory name"
assert_eq "$(yq -r '.output_mode' "$MANIFEST")" "fixed-location" \
  "spec-kit is a fixed-location driver -- Spec Kit can't be told where to write"
DECLARED_FIXED_PATH="$(yq -r '.fixed_path' "$MANIFEST")"
assert_eq "$DECLARED_FIXED_PATH" ".specify/memory/constitution.md" \
  "manifest declares the constitution path Spec Kit always writes to"
COMMAND="$(yq -r '.command' "$MANIFEST")"
[ -x "$DRIVER_DIR/$COMMAND" ] \
  && pass "the command the manifest names is executable" \
  || fail "the command the manifest names is executable"

# A fixed_path that drifts from the path run.sh actually keeps would fail
# silently at harvest time, so the two are pinned to each other here.
assert_contains "$(cat "$DRIVER_DIR/$COMMAND")" "CONSTITUTION=\"$DECLARED_FIXED_PATH\"" \
  "the path run.sh keeps is the same one the manifest declares as fixed_path"

echo ""
echo "spec-kit driver is reachable through run-driver.sh:"

REPO="$WORK/repo"
mkdir -p "$REPO"
(
  cd "$REPO"
  git init -q
  echo "hi" > README.md
  git add -A
  git -c user.email=test@example.com -c user.name=test commit -qm init
)

out_path="$WORK/out.md"
if err="$(PATH="$SPECLESS_PATH" ARCHIMEDES_DRIVERS_DIR="$ROOT/template/drivers" \
    "$RUN_DRIVER" spec-kit "$REPO" "$out_path" 2>&1 >/dev/null)"; then
  fail "a run without the specify CLI installed exits non-zero"
else
  pass "a run without the specify CLI installed exits non-zero"
fi
assert_contains "$err" "specify CLI not found on PATH" \
  "the failure is the driver's own dependency check, so the manifest resolved and the driver ran"
assert_contains "$err" "spec-kit" \
  "the failure names an install hint for Spec Kit rather than leaving the operator guessing"
assert_file_missing "$out_path" "a failed run leaves no output file"
assert_eq "$(git -C "$REPO" status --porcelain)" "" "a failed run leaves the target repo clean"

echo ""
echo "switching an instance to spec-kit is a configuration change only:"

make_origin_and_clone "$WORK" spec-kit-repo
INSTANCE="$(new_test_instance "$WORK")"

cat > "$INSTANCE/repos.yaml" <<EOF
driver: spec-kit
repos:
  - name: spec-kit-repo
    path: ../spec-kit-repo
    base_branch: main
    depends_on: []
    context_modeled_sha: null
EOF

(
  cd "$INSTANCE"
  export ARCHIMEDES_DRIVERS_DIR="$ROOT/template/drivers"
  PATH="$SPECLESS_PATH" ./scripts/context-map-all.sh
) </dev/null >"$WORK/run.log" 2>&1
run_status=$?

if [ "$run_status" -ne 0 ]; then
  pass "context-map-all.sh with driver: spec-kit fails on the missing CLI rather than succeeding by accident"
else
  fail "context-map-all.sh with driver: spec-kit fails on the missing CLI rather than succeeding by accident"
fi
log="$(cat "$WORK/run.log")"
assert_contains "$log" "specify CLI not found on PATH" \
  "naming spec-kit in repos.yaml reaches the real driver -- no orchestration change needed to switch"
case "$log" in
  *"unknown driver"*) fail "spec-kit is a driver context-map-all.sh can actually find" ;;
  *) pass "spec-kit is a driver context-map-all.sh can actually find" ;;
esac

echo ""
echo "one repo can be switched to spec-kit on its own:"

# drivers/README.md's spec-kit entry argues the map it produces is a
# different shape from the other two drivers', so which one suits a repo is
# a per-repo judgement. That's only true if the per-repo override actually
# reaches this driver.
# ARCHIMEDES_DRIVERS_DIR names one directory, so the real drivers and the
# stub the other repo stays on have to sit in the same one.
MERGED_DRIVERS="$WORK/drivers"
mkdir -p "$MERGED_DRIVERS"
cp -R "$ROOT/template/drivers/." "$MERGED_DRIVERS/"
cp -R "$HERE/fixtures/drivers/stub-ok" "$MERGED_DRIVERS/"

make_origin_and_clone "$WORK" stays-on-default
make_origin_and_clone "$WORK" switched-to-spec-kit
INSTANCE2="$(new_test_instance "$WORK")"

cat > "$INSTANCE2/repos.yaml" <<EOF
driver: stub-ok
repos:
  - name: stays-on-default
    path: ../stays-on-default
    base_branch: main
    depends_on: []
    context_modeled_sha: null
  - name: switched-to-spec-kit
    path: ../switched-to-spec-kit
    base_branch: main
    depends_on: []
    context_modeled_sha: null
    driver: spec-kit
EOF

(
  cd "$INSTANCE2"
  export ARCHIMEDES_DRIVERS_DIR="$MERGED_DRIVERS"
  PATH="$SPECLESS_PATH" ./scripts/context-map-all.sh
) </dev/null >"$WORK/override.log" 2>&1

override_log="$(cat "$WORK/override.log")"
assert_contains "$override_log" "specify CLI not found on PATH" \
  "a single repo's own driver field reaches spec-kit, so one repo can use it without the rest of the instance doing so"
assert_contains "$(cat "$WORK/stays-on-default/CONTEXT.md" 2>/dev/null)" "stub-ok saw repo" \
  "the repo that didn't override stays on the instance-wide default"

report
