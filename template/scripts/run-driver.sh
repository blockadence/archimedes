#!/usr/bin/env bash
# Invoke a driver by name against a target repo, writing its context map to
# an exact output path. This is the only thing orchestration
# (context-map-all.sh) needs to know about drivers — it never hardcodes any
# specific driver's invocation, so swapping which driver is configured never
# touches this file or context-map-all.sh. See drivers/README.md for the
# manifest format and per-mode contract.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

[ $# -eq 3 ] || { echo "usage: run-driver.sh <driver-name> <repo-path> <output-path>" >&2; exit 1; }
DRIVER_NAME="$1"; REPO_PATH="$2"; OUTPUT_PATH="$3"

DRIVER_DIR="$DRIVERS_DIR/$DRIVER_NAME"
MANIFEST="$DRIVER_DIR/driver.yaml"
[ -f "$MANIFEST" ] || { echo "unknown driver: $DRIVER_NAME (no manifest at $MANIFEST)" >&2; exit 1; }

OUTPUT_MODE="$(yq -r '.output_mode' "$MANIFEST")"
if [ "$OUTPUT_MODE" != "path-parameterized" ]; then
  echo "driver '$DRIVER_NAME' declares output_mode '$OUTPUT_MODE', which run-driver.sh doesn't support (only path-parameterized)" >&2
  exit 1
fi

COMMAND="$(yq -r '.command' "$MANIFEST")"
DRIVER_BIN="$DRIVER_DIR/$COMMAND"
[ -x "$DRIVER_BIN" ] || { echo "driver '$DRIVER_NAME' command not executable: $DRIVER_BIN" >&2; exit 1; }

REPO_PATH="$(cd "$REPO_PATH" && pwd)"
mkdir -p "$(dirname "$OUTPUT_PATH")"

"$DRIVER_BIN" "$REPO_PATH" "$OUTPUT_PATH"

[ -f "$OUTPUT_PATH" ] || { echo "driver '$DRIVER_NAME' exited 0 but did not write $OUTPUT_PATH" >&2; exit 1; }
