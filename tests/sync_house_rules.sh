#!/usr/bin/env bash
# Integration test for issue 08-house-rules-dual-delivery's sync-side half:
# sync-house-rules.sh pushes the dossier's "## House rules" section into the
# target repo as a durably committed HOUSE_RULES.md, via a branch + PR.
# `gh` is stubbed out (a fake executable on PATH) so this runs with no real
# GitHub network access, same spirit as spawn_materializes_context.sh using
# local bare repos instead of real GitHub remotes.
#
#   tests/sync_house_rules.sh
set -euo pipefail

ARCHIMEDES_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $1" >&2; exit 1; }
pass() { echo "ok - $1"; }

# --- fixture: a fake `gh`, so `gh pr create` never touches the network ----
FAKE_BIN="$TMP/bin"
mkdir -p "$FAKE_BIN"
GH_CALL_LOG="$TMP/gh_calls.log"
: > "$GH_CALL_LOG"
cat > "$FAKE_BIN/gh" <<'EOF'
#!/usr/bin/env bash
if [ "$1" = "pr" ] && [ "$2" = "create" ]; then
  echo "$*" >> "$GH_CALL_LOG"
  if [ "${FAKE_GH_PR_CREATE_FAIL:-0}" = "1" ]; then
    echo "fake gh: a pull request for branch archimedes-sync-house-rules already exists" >&2
    exit 1
  fi
  echo "https://example.invalid/pr/1"
  exit 0
fi
echo "fake gh: unhandled invocation: $*" >&2
exit 1
EOF
chmod +x "$FAKE_BIN/gh"
export GH_CALL_LOG
export PATH="$FAKE_BIN:$PATH"

# --- fixture: bare "origin" + clone, one commit on main, no HOUSE_RULES.md
ORIGIN="$TMP/target-repo.git"
CLONE="$TMP/target-repo"
git init -q --bare -b main "$ORIGIN"
git clone -q "$ORIGIN" "$CLONE"
git -C "$CLONE" -c user.email=t@t -c user.name=t commit -q --allow-empty -m init
git -C "$CLONE" push -q origin main

INSTANCE="$TMP/instance"
cp -r "$ARCHIMEDES_ROOT/template" "$INSTANCE"
chmod +x "$INSTANCE"/scripts/*.sh
cat > "$INSTANCE/repos.yaml" <<EOF
repos:
  - name: target
    path: ../target-repo
    base_branch: main
    depends_on: []
    context_modeled_sha: null
EOF
mkdir -p "$INSTANCE/repos"
cat > "$INSTANCE/repos/target.md" <<'EOF'
# target

## House rules

Never force-push to `main`.
Every migration needs a paired rollback script.

## Known gotchas
n/a
EOF

# --- 1. dry-run: shows the pending change, touches nothing ----------------
DRYRUN_OUT="$(cd "$INSTANCE" && ./scripts/sync-house-rules.sh target --dry-run)"
echo "$DRYRUN_OUT" | grep -q "Never force-push" \
  || fail "dry-run output didn't show the pending House rules content: $DRYRUN_OUT"
pass "dry-run shows the pending HOUSE_RULES.md content"

[ -f "$CLONE/HOUSE_RULES.md" ] && fail "dry-run created HOUSE_RULES.md in the local clone"
[ "$(git -C "$CLONE" rev-parse --abbrev-ref HEAD)" = "main" ] || fail "dry-run left the clone off main"
git -C "$ORIGIN" branch --list archimedes-sync-house-rules | grep -q . \
  && fail "dry-run pushed a branch to origin"
[ -s "$GH_CALL_LOG" ] && fail "dry-run called gh pr create"
pass "dry-run makes no local commit, no push, and no PR"

# --- 2. real run: commits, pushes, opens a PR ------------------------------
( cd "$INSTANCE" && ./scripts/sync-house-rules.sh target ) \
  || fail "sync-house-rules.sh exited non-zero"

git -C "$ORIGIN" branch --list archimedes-sync-house-rules | grep -q . \
  || fail "sync-house-rules.sh did not push archimedes-sync-house-rules to origin"
pass "sync-house-rules.sh pushes a durable branch to the target repo's origin"

PUSHED_CONTENT="$(git -C "$CLONE" show archimedes-sync-house-rules:HOUSE_RULES.md)"
echo "$PUSHED_CONTENT" | grep -q "Never force-push to \`main\`." \
  || fail "pushed HOUSE_RULES.md is missing expected content: $PUSHED_CONTENT"
echo "$PUSHED_CONTENT" | grep -q "Every migration needs a paired rollback script." \
  || fail "pushed HOUSE_RULES.md is missing its second house rule"
pass "the durable HOUSE_RULES.md committed to the branch matches the dossier's House rules section"

[ "$(git -C "$CLONE" rev-parse --abbrev-ref HEAD)" = "main" ] \
  || fail "sync-house-rules.sh left the local clone off its base branch"
pass "the local clone is left back on the base branch after syncing"

grep -q "pr create" "$GH_CALL_LOG" || fail "gh pr create was not called"
grep -q -- "--base main" "$GH_CALL_LOG" || fail "gh pr create wasn't given the right base branch"
grep -q -- "--head archimedes-sync-house-rules" "$GH_CALL_LOG" || fail "gh pr create wasn't given the right head branch"
pass "sync-house-rules.sh opens a PR for the durable copy via gh"

# --- 3. re-running when a PR already exists is reported, not fatal --------
: > "$GH_CALL_LOG"
FAKE_GH_PR_CREATE_FAIL=1 bash -c "cd '$INSTANCE' && ./scripts/sync-house-rules.sh target" \
  >"$TMP/rerun.log" 2>&1
RERUN_EXIT=$?
[ "$RERUN_EXIT" -eq 0 ] || { cat "$TMP/rerun.log"; fail "re-running after a PR already exists should not be fatal"; }
grep -qi "already exists\|check manually" "$TMP/rerun.log" \
  || fail "re-run didn't report the existing-PR situation: $(cat "$TMP/rerun.log")"
pass "re-running when gh reports an existing PR is surfaced, not fatal"

# --- 4. once the target's own copy matches, sync is a no-op ---------------
# Simulate the PR having merged: fast-forward main to include HOUSE_RULES.md.
git -C "$CLONE" checkout -q main
git -C "$CLONE" merge -q --ff-only archimedes-sync-house-rules
git -C "$CLONE" push -q origin main
: > "$GH_CALL_LOG"
NOOP_OUT="$(cd "$INSTANCE" && ./scripts/sync-house-rules.sh target)"
echo "$NOOP_OUT" | grep -qi "already current" || fail "sync-house-rules.sh didn't report already-current: $NOOP_OUT"
[ -s "$GH_CALL_LOG" ] && fail "an already-current sync still called gh pr create"
pass "once the target repo's HOUSE_RULES.md already matches the dossier, sync-house-rules.sh is a no-op"

echo "All sync_house_rules checks passed."
