#!/usr/bin/env bash
# Integration test for spawn.sh's worktree-materialization behavior (issue
# 02-worktree-materialization-no-commit): spawning a worktree must copy the
# unit of work's control-repo reference material into it, and that copy must
# never be able to show up in `git status`/`git add -A`, for any kind of
# artifact, without requiring any .gitignore edit in the target repo.
#
# Not vendored into instances (lives outside template/) — this is a test of
# Archimedes itself, run from a checkout of this repo:
#   tests/spawn_materializes_context.sh
set -euo pipefail

ARCHIMEDES_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=tests/gitfixture.sh
. "$ARCHIMEDES_ROOT/tests/gitfixture.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $1" >&2; exit 1; }
pass() { echo "ok - $1"; }

# --- fixture: two bare "origin"s plus clones, each with a normal .gitignore
# (two target repos, so the same slug can be spawned into both — the
# multi-repo/stacked-work pattern this instance pattern exists for).
make_target_repo() { # <name>
  make_origin_and_clone_at "$TMP/$1.git" "$TMP/$1" .gitignore $'*.log\n'
}
make_target_repo target-repo
make_target_repo target-repo2
TARGET_REPO="$TMP/target-repo"
TARGET_REPO2="$TMP/target-repo2"
GITIGNORE_BEFORE="$(cat "$TARGET_REPO/.gitignore")"

# --- fixture: an Archimedes instance pointing at both target repos --------
# path is relative to the instance root, same convention bootstrap.sh uses
# ("../<repo-name>"): instance and target repos are sibling directories.
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
  - name: target2
    path: ../target-repo2
    base_branch: main
    depends_on: []
    context_modeled_sha: null
EOF

SLUG="widget-fix"
mkdir -p "$INSTANCE/work/$SLUG"
# Two different kinds of artifact, to prove the mechanism is content-agnostic.
echo "# Ticket: widgets are broken" > "$INSTANCE/work/$SLUG/ticket.md"
printf '\x89PNG\r\n\x1a\nfakebinarydata' > "$INSTANCE/work/$SLUG/mockup.png"

# --- act: spawn a worktree ------------------------------------------------
( cd "$INSTANCE" && ./scripts/spawn.sh "$SLUG" target >"$TMP/spawn_out.log" 2>&1 ) \
  || { cat "$TMP/spawn_out.log"; fail "spawn.sh exited non-zero"; }

WT="$TARGET_REPO-worktrees/$SLUG"
[ -d "$WT" ] || fail "worktree was not created at $WT"

# 1. Spawning copies the control-repo artifact into a conventional location.
[ -f "$WT/.archimedes/ticket.md" ] || fail "ticket.md was not materialized into the worktree"
[ -f "$WT/.archimedes/mockup.png" ] || fail "mockup.png was not materialized into the worktree"
diff -q "$INSTANCE/work/$SLUG/ticket.md" "$WT/.archimedes/ticket.md" >/dev/null \
  || fail "materialized ticket.md content diverged from the source"
pass "worktree spawn materializes work/<slug>/ into .archimedes/"

# 2. git status / git add -A never surface it, for either artifact kind.
STATUS_OUT="$(git -C "$WT" status --porcelain)"
echo "$STATUS_OUT" | grep -q '.archimedes' && fail "git status surfaced .archimedes: $STATUS_OUT"
pass "git status does not surface .archimedes/"

git -C "$WT" add -A
ADDED="$(git -C "$WT" status --porcelain)"
echo "$ADDED" | grep -q '.archimedes' && fail "git add -A staged .archimedes: $ADDED"
pass "git add -A does not stage .archimedes/"

# 3. No manual .gitignore edit in the target repo was needed or made.
[ "$(cat "$TARGET_REPO/.gitignore")" = "$GITIGNORE_BEFORE" ] \
  || fail "target repo's tracked .gitignore was modified"
[ "$(git -C "$WT" show HEAD:.gitignore)" = "$GITIGNORE_BEFORE" ] \
  || fail "worktree's committed .gitignore differs from target repo's"
pass "target repo's tracked .gitignore is untouched"

# 3b. Archimedes' own cross-repo bookkeeping (status.md — by now containing
# the first worktree's local path, since this slug spans two repos) is not
# reference material and must not leak into a second repo's worktree.
[ -f "$INSTANCE/work/$SLUG/status.md" ] || fail "expected status.md to exist after first spawn"
( cd "$INSTANCE" && ./scripts/spawn.sh "$SLUG" target2 >"$TMP/spawn_out2.log" 2>&1 ) \
  || { cat "$TMP/spawn_out2.log"; fail "spawn.sh exited non-zero for the second repo"; }
WT2="$TARGET_REPO2-worktrees/$SLUG"
[ -f "$WT2/.archimedes/ticket.md" ] || fail "ticket.md was not materialized into the second worktree"
[ -f "$WT2/.archimedes/status.md" ] && fail "status.md (bookkeeping, not reference material) leaked into the second worktree"
pass "status.md is excluded from the materialized context, even for a second worktree of the same slug"

# 4. Removing/pruning the worktree cleans up without manual gitignore edits.
git -C "$TARGET_REPO" worktree remove "$WT" --force
[ -d "$WT" ] && fail "worktree directory still exists after removal"
[ "$(cat "$TARGET_REPO/.gitignore")" = "$GITIGNORE_BEFORE" ] \
  || fail "pruning required/caused a .gitignore edit in the target repo"
pass "pruning the worktree cleans up .archimedes/ with it, no gitignore edits needed"

echo "All spawn_materializes_context checks passed."
