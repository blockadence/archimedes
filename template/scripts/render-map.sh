#!/usr/bin/env bash
# Regenerate WORKSPACE-MAP.md's "## Repos" block from repos.yaml.
# "## Relationships" below it is hand-written and untouched here.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

MAP="$ROOT/WORKSPACE-MAP.md"
[ -f "$MAP" ] || printf '# Workspace Map\n\n## Repos\n\n## Relationships\n' > "$MAP"

block=$(yq -r '.repos[] | "- [" + .name + "](" + .path + ") — base: `" + .base_branch + "`. Dossier: [repos/" + .name + ".md](./repos/" + .name + ".md)"' "$REPOS_YAML")

# block is passed through the environment rather than -v: BSD awk (macOS's
# system awk) rejects a newline inside a -v assignment, so -v worked for a
# one-repo instance and failed with "newline in string" from the second repo
# onward, taking bootstrap.sh's final render-map step down with it.
BLOCK="$block" awk '
  /^## Repos/ { print; print ""; print ENVIRON["BLOCK"]; skip=1; next }
  /^## Relationships/ { skip=0 }
  skip && $0 != "" { next }
  { print }
' "$MAP" > "$MAP.tmp" && mv "$MAP.tmp" "$MAP"
