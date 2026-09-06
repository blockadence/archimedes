#!/usr/bin/env bash
# Push the canonical PR/issue templates (template/scaffolding/) into every
# tracked repo's .github/ via a pull request. Thin wrapper around
# multi-gitter for the actual fan-out — no custom multi-repo PR engine here.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require multi-gitter

SCAFFOLD_DIR="$ROOT/scaffolding"
DRY_RUN=0; REPO_FILTER=""
for arg in "$@"; do
  [ "$arg" = "--dry-run" ] && DRY_RUN=1 || REPO_FILTER="$arg"
done

repo_args=()
while IFS= read -r name; do
  [ -n "$REPO_FILTER" ] && [ "$name" != "$REPO_FILTER" ] && continue
  repo_args+=(-R "$(gh_slug "$name")")
done < <(yq -r '.repos[].name' "$REPOS_YAML")

if [ "${#repo_args[@]}" -eq 0 ]; then
  echo "No repos matched${REPO_FILTER:+ '$REPO_FILTER'} in $REPOS_YAML." >&2
  exit 1
fi

MOD_SCRIPT="$(mktemp)"
trap 'rm -f "$MOD_SCRIPT"' EXIT
cat > "$MOD_SCRIPT" <<'EOF'
#!/usr/bin/env bash
# Run by multi-gitter with cwd set to the root of each cloned repo.
set -euo pipefail
mkdir -p .github/ISSUE_TEMPLATE
cp "$SCAFFOLD_DIR/PULL_REQUEST_TEMPLATE.md" .github/PULL_REQUEST_TEMPLATE.md
cp "$SCAFFOLD_DIR"/ISSUE_TEMPLATE/*.yml .github/ISSUE_TEMPLATE/
EOF
chmod +x "$MOD_SCRIPT"
export SCAFFOLD_DIR

flags=(
  --branch archimedes-sync-templates
  --commit-message "Sync canonical PR/issue templates from Archimedes"
  --pr-title "Sync canonical PR/issue templates"
  --pr-body "Introduces/updates this repo's .github/ pull request and issue templates from the canonical set maintained in Archimedes."
  --token "$(gh auth token)"
)
[ "$DRY_RUN" -eq 1 ] && flags+=(--dry-run)

multi-gitter run "$MOD_SCRIPT" "${repo_args[@]}" "${flags[@]}"
