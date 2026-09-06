#!/usr/bin/env bash
# End-to-end: a real (throwaway) Archimedes instance, with ARCHIMEDES_DRIVER
# set, drives scripts/context-map-all.sh unattended — no `read` prompt, no
# hardcoded knowledge of which driver is configured — and the resulting
# context file lands exactly where told in the target repo.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "context-map-all.sh driver wiring:"

# A bare "origin" plus a clone, so fetch/rev-parse work with no network.
git init -q --bare "$WORK/origin.git"
git clone -q "$WORK/origin.git" "$WORK/target-repo"
(
  cd "$WORK/target-repo"
  git checkout -q -b main
  echo "hi" > README.md
  git add -A
  git -c user.email=test@example.com -c user.name=test commit -qm init
  git push -q -u origin main
)

# An instance is just scripts/ + repos.yaml at its root (matches what
# init.sh/update-from-archimedes.sh vendor into a real instance).
mkdir -p "$WORK/instance/scripts"
cp "$ROOT/template/scripts/"*.sh "$WORK/instance/scripts/"
chmod +x "$WORK/instance/scripts/"*.sh
cat > "$WORK/instance/repos.yaml" <<EOF
repos:
  - name: target-repo
    path: ../target-repo
    base_branch: main
    depends_on: []
    context_modeled_sha: null
EOF

(
  cd "$WORK/instance"
  export ARCHIMEDES_DRIVER=stub-ok
  export ARCHIMEDES_DRIVERS_DIR="$HERE/fixtures/drivers"
  ./scripts/context-map-all.sh
) </dev/null >"$WORK/run.log" 2>&1
run_status=$?

if [ "$run_status" -eq 0 ]; then
  pass "context-map-all.sh runs unattended (no read prompt) when a driver is configured"
else
  fail "context-map-all.sh runs unattended (no read prompt) when a driver is configured"
  cat "$WORK/run.log" >&2
fi

assert_file_exists "$WORK/target-repo/CONTEXT.md" "context file lands exactly at <repo>/CONTEXT.md"
assert_contains "$(cat "$WORK/target-repo/CONTEXT.md" 2>/dev/null)" "stub-ok saw repo" "context file content came from the configured driver"

recorded_sha="$(yq -r '.repos[] | select(.name == "target-repo") | .context_modeled_sha' "$WORK/instance/repos.yaml")"
current_sha="$(git -C "$WORK/target-repo" rev-parse origin/main)"
assert_eq "$recorded_sha" "$current_sha" "repos.yaml records the mapped repo as current"

report
