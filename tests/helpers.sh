#!/usr/bin/env bash
# Minimal assertion + e2e-fixture helpers shared by tests/*.sh. Not a
# framework — this repo is plain bash throughout, so tests stay plain bash
# too.
set -uo pipefail

HELPERS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TESTS_REPO_ROOT="$(cd "$HELPERS_DIR/.." && pwd)"

TESTS_RUN=0
TESTS_FAILED=0

pass() { TESTS_RUN=$((TESTS_RUN + 1)); echo "  ok: $1"; }
fail() { TESTS_RUN=$((TESTS_RUN + 1)); TESTS_FAILED=$((TESTS_FAILED + 1)); echo "  FAIL: $1" >&2; }

assert_eq() { # <actual> <expected> <label>
  if [ "$1" = "$2" ]; then pass "$3"; else fail "$3 (expected [$2], got [$1])"; fi
}

assert_contains() { # <haystack> <needle> <label>
  case "$1" in
    *"$2"*) pass "$3" ;;
    *) fail "$3 (expected to contain [$2], got [$1])" ;;
  esac
}

assert_file_exists() { # <path> <label>
  [ -f "$1" ] && pass "$2" || fail "$2 (no file at $1)"
}

assert_file_missing() { # <path> <label>
  [ -f "$1" ] && fail "$2 (unexpectedly found $1)" || pass "$2"
}

# Directories need their own assertions rather than reusing the file ones,
# because `git status` can't stand in for them: git doesn't track
# directories, so a scaffolded toolchain left behind in empty ones is
# invisible to every porcelain check.
assert_dir_exists() { # <path> <label>
  [ -d "$1" ] && pass "$2" || fail "$2 (no directory at $1)"
}

assert_dir_missing() { # <path> <label>
  [ -d "$1" ] && fail "$2 (unexpectedly found $1/)" || pass "$2"
}

report() { # call at end of each test file
  echo "$TESTS_RUN run, $TESTS_FAILED failed"
  [ "$TESTS_FAILED" -eq 0 ]
}

# A bare "origin" plus a clone with one commit pushed to main, so an
# e2e test's fetch/rev-parse work with no network. <work-dir> <name> ->
# creates <work-dir>/<name> (the clone tests operate on) and
# <work-dir>/<name>-origin.git (the bare remote).
make_origin_and_clone() {
  local work="$1" name="$2"
  git init -q --bare "$work/$name-origin.git"
  git clone -q "$work/$name-origin.git" "$work/$name"
  (
    cd "$work/$name"
    git checkout -q -b main
    echo "hi" > README.md
    git add -A
    git -c user.email=test@example.com -c user.name=test commit -qm init
    git push -q -u origin main
  )
}

# Scaffold a throwaway Archimedes instance under <work-dir>/instance: just
# scripts/ (vendored from template/scripts, matching what
# init.sh/update-from-archimedes.sh vendor into a real instance) — the
# caller still writes its own repos.yaml. Echoes the instance path.
new_test_instance() {
  local work="$1"
  mkdir -p "$work/instance/scripts"
  cp "$TESTS_REPO_ROOT/template/scripts/"*.sh "$work/instance/scripts/"
  chmod +x "$work/instance/scripts/"*.sh
  echo "$work/instance"
}

# A throwaway one-commit git repo with just enough of a domain in it for a
# context-mapping driver to have something to say about. Shared by the
# driver e2e tests so they're all pointed at the same target -- what varies
# between them should be the driver, not the repo. <path> -> creates it.
make_widget_repo() {
  local repo="$1"
  mkdir -p "$repo/src"
  cat > "$repo/src/index.js" <<'EOF'
// A tiny widget-catalog service: Widgets have a name and a price.
class Widget {
  constructor(name, priceCents) {
    this.name = name;
    this.priceCents = priceCents;
  }
}
module.exports = { Widget };
EOF
  (
    cd "$repo"
    git init -q
    git add -A
    git -c user.email=test@example.com -c user.name=test commit -qm init
  )
}
