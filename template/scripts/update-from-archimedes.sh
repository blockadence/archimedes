#!/usr/bin/env bash
# Re-vendor the latest scripts/ from a local Archimedes checkout into this
# instance. Doesn't touch repos.yaml, WORKSPACE-MAP.md, repos/, or work/.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARCHIMEDES_SRC="${1:?usage: update-from-archimedes.sh <path-to-archimedes-checkout>}"

cp -r "$ARCHIMEDES_SRC/template/scripts/." "$ROOT/scripts/"
chmod +x "$ROOT"/scripts/*.sh
echo "scripts/ refreshed from $ARCHIMEDES_SRC"
