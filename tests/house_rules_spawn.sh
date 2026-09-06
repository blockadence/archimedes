#!/usr/bin/env bash
# Integration test for issue 08-house-rules-dual-delivery's spawn-side half:
# spawning a worktree must inject an ephemeral copy of the target repo's
# house rules (dossier's "## House rules" section) automatically, even when
# the slug being spawned has no other work/<slug> reference material — and
# must inject nothing when the repo has no house rules recorded.
#
#   tests/house_rules_spawn.sh
set -euo pipefail

ARCHIMEDES_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $1" >&2; exit 1; }
pass() { echo "ok - $1"; }

make_target_repo() { # <name>
  local name="$1" origin="$TMP/$1.git" clone="$TMP/$1"
  git init -q --bare -b main "$origin"
  git clone -q "$origin" "$clone"
  git -C "$clone" commit -q --allow-empty -m init
  git -C "$clone" push -q origin main
}
make_target_repo has-rules
make_target_repo no-rules

INSTANCE="$TMP/instance"
cp -r "$ARCHIMEDES_ROOT/template" "$INSTANCE"
chmod +x "$INSTANCE"/scripts/*.sh

cat > "$INSTANCE/repos.yaml" <<EOF
repos:
  - name: has-rules
    path: ../has-rules
    base_branch: main
    depends_on: []
    context_modeled_sha: null
  - name: no-rules
    path: ../no-rules
    base_branch: main
    depends_on: []
    context_modeled_sha: null
EOF

mkdir -p "$INSTANCE/repos"
cat > "$INSTANCE/repos/has-rules.md" <<'EOF'
# has-rules

## House rules

Never rebase a shared branch.
All schema changes go through the migration tool, no exceptions.

## Known gotchas
n/a
EOF
cat > "$INSTANCE/repos/no-rules.md" <<'EOF'
# no-rules

## House rules

## Known gotchas
n/a
EOF

# --- act: spawn into the repo with house rules, no work/<slug> content ----
SLUG="quiet-fix"
( cd "$INSTANCE" && ./scripts/spawn.sh "$SLUG" has-rules >"$TMP/spawn1.log" 2>&1 ) \
  || { cat "$TMP/spawn1.log"; fail "spawn.sh exited non-zero for has-rules"; }

WT="$TMP/has-rules-worktrees/$SLUG"
[ -f "$WT/.archimedes/HOUSE_RULES.md" ] || fail "HOUSE_RULES.md was not materialized for a repo with house rules"
EXPECTED="$(printf 'Never rebase a shared branch.\nAll schema changes go through the migration tool, no exceptions.\n')"
ACTUAL="$(cat "$WT/.archimedes/HOUSE_RULES.md")"
[ "$ACTUAL" = "$(printf '%s' "$EXPECTED")" ] || fail "materialized HOUSE_RULES.md content diverged from the dossier (got: $ACTUAL)"
pass "spawning injects the dossier's House rules section into .archimedes/HOUSE_RULES.md, with no work/<slug> content present"

STATUS_OUT="$(git -C "$WT" status --porcelain)"
echo "$STATUS_OUT" | grep -q '.archimedes' && fail "git status surfaced .archimedes: $STATUS_OUT"
pass "the ephemeral house-rules copy stays outside git status, same as other .archimedes/ content"

# --- act: spawn into a repo with no house rules recorded -------------------
# A different slug than above, so the first spawn's status.md bookkeeping
# (copied into .archimedes/ and then stripped back out, per
# materialize_worktree_context) can't be mistaken for house-rules content.
SLUG2="another-fix"
( cd "$INSTANCE" && ./scripts/spawn.sh "$SLUG2" no-rules >"$TMP/spawn2.log" 2>&1 ) \
  || { cat "$TMP/spawn2.log"; fail "spawn.sh exited non-zero for no-rules"; }

WT2="$TMP/no-rules-worktrees/$SLUG2"
[ -e "$WT2/.archimedes" ] && fail "an empty .archimedes/ was created for a repo with no house rules and no work/<slug> content"
pass "spawning a repo with no house rules and no other reference material creates no .archimedes/ at all"

echo "All house_rules_spawn checks passed."
