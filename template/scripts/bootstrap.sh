#!/usr/bin/env bash
# Discover org repos via `gh`, clone what's missing, scaffold repos.yaml +
# per-repo dossier stubs, then regenerate WORKSPACE-MAP.md.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

ORG="${1:?usage: bootstrap.sh <github-org> [--include-archived]}"
INCLUDE_ARCHIVED="${2:-}"

gh repo list "$ORG" --limit 300 \
  --json name,sshUrl,defaultBranchRef,isArchived,isFork > /tmp/archimedes-repos.json

jq_filter='.[] | select(.isFork == false)'
[ "$INCLUDE_ARCHIVED" = "--include-archived" ] || jq_filter+=' | select(.isArchived == false)'
jq -c "$jq_filter" /tmp/archimedes-repos.json > /tmp/archimedes-repos.filtered.json

[ -f "$REPOS_YAML" ] || echo "repos: []" > "$REPOS_YAML"
new_count=0

while IFS= read -r repo; do
  name=$(jq -r '.name' <<<"$repo")
  url=$(jq -r '.sshUrl' <<<"$repo")
  base=$(jq -r '.defaultBranchRef.name // "main"' <<<"$repo")
  path="../$name"

  if ! yq -e ".repos[] | select(.name == \"$name\")" "$REPOS_YAML" >/dev/null 2>&1; then
    yq -i ".repos += [{\"name\": \"$name\", \"path\": \"$path\", \"base_branch\": \"$base\", \"depends_on\": []}]" "$REPOS_YAML"
    new_count=$((new_count + 1))
  fi

  target="$ROOT/$path"
  [ -d "$target" ] || { echo "cloning $name"; git clone "$url" "$target"; }

  dossier="$ROOT/repos/$name.md"
  if [ ! -f "$dossier" ]; then
    mkdir -p "$ROOT/repos"
    cat > "$dossier" <<EOF
# $name

**Path:** $path
**Base branch:** $base
**Depends on:** TBD
**Depended on by:** TBD

## Branching
TBD, fill in during the domain-modeling / dossier pass.

## Release procedure
TBD

## Known gotchas
TBD
EOF
  fi
done < /tmp/archimedes-repos.filtered.json

echo "New repos added: $new_count"
"$(dirname "${BASH_SOURCE[0]}")/render-map.sh"
