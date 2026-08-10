#!/usr/bin/env bash
# Regenerate WORKSPACE-MAP.md's "## Repos" block from repos.yaml.
# "## Relationships" below it is hand-written and untouched here.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

MAP="$ROOT/WORKSPACE-MAP.md"
[ -f "$MAP" ] || printf '# Workspace Map\n\n## Repos\n\n## Relationships\n' > "$MAP"

block=$(yq -r '.repos[] | "- [" + .name + "](" + .path + ") — base: `" + .base_branch + "`. Dossier: [repos/" + .name + ".md](./repos/" + .name + ".md)"' "$REPOS_YAML")

awk -v block="$block" '
  /^## Repos/ { print; print ""; print block; skip=1; next }
  /^## Relationships/ { skip=0 }
  skip && $0 != "" { next }
  { print }
' "$MAP" > "$MAP.tmp" && mv "$MAP.tmp" "$MAP"
