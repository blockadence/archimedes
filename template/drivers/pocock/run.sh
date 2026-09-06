#!/usr/bin/env bash
# Fixed-location driver: `run.sh <repo-path>`.
#
# Runs a headless `claude -p` session inside <repo-path>, instructed to use
# the domain-modeling skill (https://github.com/mattpocock/skills) to build
# or refresh that repo's CONTEXT.md from its codebase. The skill always
# writes CONTEXT.md at the root of whatever repo it's invoked in and can't
# be pointed at an explicit output path -- run-driver.sh harvests
# <repo-path>/CONTEXT.md after this exits (see driver.yaml's fixed_path).
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: run.sh <repo-path>" >&2; exit 1; }
REPO_PATH="$1"

command -v claude >/dev/null 2>&1 || {
  echo "claude CLI not found on PATH" >&2
  exit 1
}

PROMPT="Use the domain-modeling skill to build or refresh this repo's CONTEXT.md by reading the codebase. Resolve every term you can directly from the code; do not ask questions, since no one is here to answer them. Write only CONTEXT.md -- this repo is harvested by moving exactly that one file elsewhere, so do not create or modify any other file (no ADRs, no docs/adr/, nothing else), even if the skill's own criteria would otherwise call for one. When CONTEXT.md is up to date, stop."

(
  cd "$REPO_PATH"
  claude -p \
    --tools "Read,Glob,Grep,Write,Edit" \
    --permission-mode bypassPermissions \
    "$PROMPT"
) >&2

[ -f "$REPO_PATH/CONTEXT.md" ] || { echo "domain-modeling skill did not produce $REPO_PATH/CONTEXT.md" >&2; exit 1; }
