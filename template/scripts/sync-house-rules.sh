#!/usr/bin/env bash
# Push one repo's house rules (the "## House rules" section of its dossier,
# repos/<repo>.md) into that repo as a durably committed HOUSE_RULES.md, via
# a pull request. Unlike sync-templates.sh this isn't a multi-gitter
# fan-out: the content is specific to one repo, not identical across every
# tracked repo, so it operates directly on that repo's own local clone
# (already present from bootstrap.sh) with plain git + gh — no new
# dependency beyond what lib.sh already requires.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

[ $# -ge 1 ] || { echo "usage: sync-house-rules.sh <repo> [--dry-run]"; exit 1; }
REPO="$1"; shift
DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

repo_exists "$REPO" || { echo "unknown repo: $REPO" >&2; exit 1; }
REPO_PATH="$(repo_path "$REPO")"
[ -d "$REPO_PATH" ] || { echo "unknown repo checkout: $REPO_PATH (run bootstrap.sh first)" >&2; exit 1; }
BASE_BRANCH="$(repo_field "$REPO" base_branch)"

RULES="$(house_rules_content "$REPO")"
[ -n "$RULES" ] || { echo "no house rules recorded for $REPO ($DOSSIER_DIR/$REPO.md); nothing to sync." >&2; exit 1; }
NEW_CONTENT="$(printf '# House rules\n\n%s\n' "$RULES")"

git -C "$REPO_PATH" fetch origin
git -C "$REPO_PATH" checkout "$BASE_BRANCH"
git -C "$REPO_PATH" pull --ff-only origin "$BASE_BRANCH"

TARGET_FILE="$REPO_PATH/HOUSE_RULES.md"
OLD_FILE="$TARGET_FILE"
[ -f "$OLD_FILE" ] || OLD_FILE=/dev/null

if [ "$(cat "$OLD_FILE" 2>/dev/null || true)" = "$NEW_CONTENT" ]; then
  echo "$REPO: HOUSE_RULES.md already current on $BASE_BRANCH."
  exit 0
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo "$REPO: would update HOUSE_RULES.md:"
  diff -u "$OLD_FILE" - <<<"$NEW_CONTENT" || true
  exit 0
fi

BRANCH="archimedes-sync-house-rules"
git -C "$REPO_PATH" checkout -B "$BRANCH"
printf '%s' "$NEW_CONTENT" > "$TARGET_FILE"
git -C "$REPO_PATH" add HOUSE_RULES.md
git -C "$REPO_PATH" commit -q -m "Sync house rules from Archimedes"
git -C "$REPO_PATH" push -q -u origin "$BRANCH" --force-with-lease

if ! gh pr create --repo "$(gh_slug "$REPO")" --base "$BASE_BRANCH" --head "$BRANCH" \
  --title "Sync house rules from Archimedes" \
  --body "Updates HOUSE_RULES.md from this repo's dossier in Archimedes ($DOSSIER_DIR/$REPO.md). Edit the dossier, not this file, and re-run sync-house-rules.sh $REPO."; then
  echo "$REPO: PR create failed or a PR for $BRANCH already exists; check manually." >&2
fi

git -C "$REPO_PATH" checkout "$BASE_BRANCH"
