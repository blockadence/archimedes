#!/usr/bin/env bash
# One-time scaffold helper: wire a repo up to its declared convention pack
# (repos.yaml's `convention_pack` field) by adding whatever dependency or
# plugin reference that repo's build tool needs to pull in the shared
# config artifact. Idempotent: re-running against a repo already on the
# convention is a no-op. This is scaffolding, not ongoing sync — after this
# runs, the repo owns that reference like any other dependency; nothing
# here pushes updates back into it later.
#
# Dispatches on the pack's `build_tool` field so adding a second
# language/build tool later is one new function, not a rewrite. See
# convention-packs/README.md.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

PACKS_DIR="$ROOT/convention-packs"

REPO="${1:?usage: apply-convention-pack.sh <repo>}"
repo_exists "$REPO" || { echo "unknown repo: $REPO (not in repos.yaml)" >&2; exit 1; }
REPO_PATH="$(repo_path "$REPO")"
[ -d "$REPO_PATH" ] || { echo "$REPO is in repos.yaml but not cloned yet (run bootstrap.sh)." >&2; exit 1; }

PACK_NAME="$(repo_field "$REPO" convention_pack)"
[ -n "$PACK_NAME" ] && [ "$PACK_NAME" != "null" ] || {
  echo "$REPO has no convention_pack set in repos.yaml, nothing to do." >&2
  exit 1
}

PACK_FILE="$PACKS_DIR/$PACK_NAME.yaml"
[ -f "$PACK_FILE" ] || { echo "unknown convention pack: $PACK_NAME (no $PACK_FILE)" >&2; exit 1; }

BUILD_TOOL="$(yq -r '.build_tool' "$PACK_FILE")"

# Applies the pack via the buildscript-classpath idiom (`buildscript {
# dependencies { classpath "group:id:version" } }` + `apply plugin: "id"`)
# rather than the `plugins {}` DSL: the plugins {} DSL only resolves a
# plugin that's already on the Gradle Plugin Portal or that the target
# repo's settings.gradle already points `pluginManagement` at. Applying by
# binary coordinate works for a privately-published shared artifact without
# assuming either, and it's what actually consumes the pack's own
# artifact.{group,id,version} fields.
apply_gradle() {
  local group id version plugin_id coordinate build_file is_kts=0
  group="$(yq -r '.artifact.group' "$PACK_FILE")"
  id="$(yq -r '.artifact.id' "$PACK_FILE")"
  version="$(yq -r '.artifact.version' "$PACK_FILE")"
  plugin_id="$(yq -r '.gradle.plugin_id' "$PACK_FILE")"
  coordinate="$group:$id:$version"

  if [ -f "$REPO_PATH/build.gradle.kts" ]; then
    build_file="$REPO_PATH/build.gradle.kts"; is_kts=1
  elif [ -f "$REPO_PATH/build.gradle" ]; then
    build_file="$REPO_PATH/build.gradle"
  else
    echo "no build.gradle(.kts) found at $REPO_PATH, not a Gradle project yet." >&2
    exit 1
  fi

  if grep -qF "$plugin_id" "$build_file"; then
    echo "$REPO is already on $PACK_NAME ($plugin_id found in $(basename "$build_file"))."
    return 0
  fi

  if grep -qE '^[[:space:]]*buildscript[[:space:]]*\{' "$build_file"; then
    echo "$(basename "$build_file") already has a buildscript {} block." >&2
    echo "Add this dependency inside its dependencies {} block by hand, plus the apply line near the top:" >&2
    if [ "$is_kts" -eq 1 ]; then
      echo "  classpath(\"$coordinate\")" >&2
      echo "  apply(plugin = \"$plugin_id\")" >&2
    else
      echo "  classpath '$coordinate'" >&2
      echo "  apply plugin: '$plugin_id'" >&2
    fi
    exit 1
  fi

  local classpath_line apply_line block
  if [ "$is_kts" -eq 1 ]; then
    classpath_line="        classpath(\"$coordinate\")"
    apply_line="apply(plugin = \"$plugin_id\")"
  else
    classpath_line="        classpath '$coordinate'"
    apply_line="apply plugin: '$plugin_id'"
  fi
  # Captured via $(); command substitution strips trailing newlines, so the
  # blank-line separator is added back at the print site below instead.
  block="$(printf 'buildscript {\n    dependencies {\n%s\n    }\n}\n%s' "$classpath_line" "$apply_line")"

  { printf '%s\n\n' "$block"; cat "$build_file"; } > "$build_file.tmp" && mv "$build_file.tmp" "$build_file"

  echo "Added $coordinate (plugin $plugin_id) to $(basename "$build_file"). Review the diff and commit it in $REPO."
}

case "$BUILD_TOOL" in
  gradle) apply_gradle ;;
  *)
    echo "no scaffold logic yet for build_tool '$BUILD_TOOL' (pack: $PACK_NAME)." >&2
    echo "add a case to apply-convention-pack.sh — see convention-packs/README.md." >&2
    exit 1
    ;;
esac
