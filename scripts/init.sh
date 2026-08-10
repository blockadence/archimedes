#!/usr/bin/env bash
# Scaffold a new instance from the Archimedes template, with fresh git
# history so instance-specific (possibly sensitive) content never lives in
# this repo's own history.
set -euo pipefail
ARCHIMEDES_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

NAME="${1:?usage: init.sh <instance-name> <dest-parent-dir>}"
DEST_PARENT="${2:?usage: init.sh <instance-name> <dest-parent-dir>}"
DEST="$DEST_PARENT/$NAME"

[ -d "$DEST" ] && { echo "$DEST already exists" >&2; exit 1; }

cp -r "$ARCHIMEDES_ROOT/template" "$DEST"
chmod +x "$DEST"/scripts/*.sh
git -C "$DEST" init -q
git -C "$DEST" add -A
git -C "$DEST" commit -q -m "Scaffold $NAME from Archimedes template"

echo "Instance ready at $DEST"
echo "Next: cd $DEST && scripts/bootstrap.sh <github-org>"
