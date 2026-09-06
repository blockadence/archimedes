#!/usr/bin/env bash
# Unit tests for the dossier-side half of issue 08-house-rules-dual-delivery:
# a dossier's "## House rules" section is distinct from "## Known gotchas",
# and lib.sh's house_rules_content() extracts it correctly. No network
# dependency (unlike bootstrap.sh itself, which calls out to `gh repo list`)
# since write_dossier_stub/house_rules_content are plain file/string logic.
#
#   tests/house_rules_dossier.sh
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ARCHIMEDES_ROOT="$(cd "$HERE/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

INSTANCE="$TMP/instance"
cp -r "$ARCHIMEDES_ROOT/template" "$INSTANCE"
chmod +x "$INSTANCE"/scripts/*.sh
echo "repos: []" > "$INSTANCE/repos.yaml"

source "$INSTANCE/scripts/lib.sh"

# --- write_dossier_stub: distinct, ordered sections ------------------------
write_dossier_stub "widget-service" "../widget-service" "main"
DOSSIER="$DOSSIER_DIR/widget-service.md"
assert_file_exists "$DOSSIER" "write_dossier_stub creates a dossier"

CONTENT="$(cat "$DOSSIER")"
assert_contains "$CONTENT" "## House rules" "dossier stub has a House rules heading"
assert_contains "$CONTENT" "## Known gotchas" "dossier stub has a Known gotchas heading"

HOUSE_LINE=$(grep -n '^## House rules' "$DOSSIER" | cut -d: -f1)
GOTCHAS_LINE=$(grep -n '^## Known gotchas' "$DOSSIER" | cut -d: -f1)
if [ "$HOUSE_LINE" -lt "$GOTCHAS_LINE" ]; then
  pass "House rules section precedes Known gotchas, as two separate sections"
else
  fail "House rules section did not precede Known gotchas"
fi

# Re-running must not clobber a hand-edited dossier.
printf '\nEdited by hand.\n' >> "$DOSSIER"
write_dossier_stub "widget-service" "../widget-service" "main"
assert_contains "$(cat "$DOSSIER")" "Edited by hand." "write_dossier_stub does not overwrite an existing dossier"

# --- house_rules_content: extraction ---------------------------------------
assert_contains "$(house_rules_content "widget-service")" "Mandated decisions" \
  "house_rules_content extracts the stub's House rules body"

cat > "$DOSSIER_DIR/custom.md" <<'EOF'
# custom

## Branching
n/a

## House rules

Never force-push to `main`.
Every migration needs a paired rollback script.

## Known gotchas
The staging DB lags prod by a day.
EOF
EXPECTED="$(printf 'Never force-push to `main`.\nEvery migration needs a paired rollback script.')"
assert_eq "$(house_rules_content "custom")" "$EXPECTED" \
  "house_rules_content extracts exactly the House rules section, trimmed, excluding neighboring sections"

cat > "$DOSSIER_DIR/no-rules.md" <<'EOF'
# no-rules

## Known gotchas
Nothing special.
EOF
assert_eq "$(house_rules_content "no-rules")" "" \
  "house_rules_content is empty when the dossier has no House rules section"

assert_eq "$(house_rules_content "does-not-exist")" "" \
  "house_rules_content is empty when the repo has no dossier at all"

report