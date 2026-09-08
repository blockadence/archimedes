#!/usr/bin/env bash
# Minimal assertion + e2e-fixture helpers shared by tests/*.sh. Not a
# framework — this repo is plain bash throughout, so tests stay plain bash
# too.
set -uo pipefail

HELPERS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TESTS_REPO_ROOT="$(cd "$HELPERS_DIR/.." && pwd)"

# shellcheck source=tests/gitfixture.sh
. "$HELPERS_DIR/gitfixture.sh"

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

assert_not_contains() { # <haystack> <needle> <label>
  case "$1" in
    *"$2"*) fail "$3 (expected not to contain [$2], got [$1])" ;;
    *) pass "$3" ;;
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
  make_origin_and_clone_at "$work/$name-origin.git" "$work/$name" README.md $'hi\n'
}

# The compiled CLI these tests drive. An instance carries no scripts of its
# own any more, so every test that acts on one acts through this binary —
# the same one an operator installs. It is built into archimedes at the
# repo root, the path .gitignore already covers, so a whole suite run shares
# one build instead of each file making its own; `go build` no-ops when it
# is current.
ARCHIMEDES_BIN="$TESTS_REPO_ROOT/archimedes"

build_archimedes() {
  ( cd "$TESTS_REPO_ROOT" && go build -o archimedes ./cmd/archimedes ) || {
    echo "could not build the archimedes CLI" >&2
    return 1
  }
}

# The block of lines nested under a header line in a YAML file --
# everything indented more deeply than the header, up to the first line
# that dedents back to it or past it. Blank lines don't end a block.
#
# Several test files read the workflows to pin guarantees that live nowhere
# but there -- the release waits on the tests (ci_gates_release.sh), the
# published assets are attested (release_provenance.sh), the billed suite is
# not reachable from a fork (ci_runs_live_drivers.sh). They all need the
# same thing of a workflow file, and reading it several ways would be
# several answers to the same question.
#
# <file> <header-line>, the header given exactly as it appears, indent and
# trailing colon included: `yaml_block wf.yml "  release:"`.
yaml_block() {
  awk -v header="$2" '
    BEGIN {
      match(header, /^[ ]*/)
      header_indent = RLENGTH
    }
    !inblock { if ($0 == header) inblock = 1; next }
    /^[[:space:]]*$/ { print; next }
    { match($0, /^[ ]*/) }
    RLENGTH <= header_indent { inblock = 0; next }
    { print }
  ' "$1"
}

# One top-level scalar field out of a driver manifest — enough for the flat
# key/value manifests drivers actually ship, and it reads nothing nested.
# Deliberately not yq: retiring the vendored scripts took yq off the list of
# things an operator has to install, and a test suite that still needed it
# would put it back. <manifest> <field>
manifest_field() {
  sed -n "s/^$2: *//p" "$1"
}

# One job's body out of a GitHub workflow: everything indented under
# `  <name>:` up to the next job. Enough structure for the three files that
# read workflows -- ci_gates_release.sh, release_provenance.sh and
# ci_runs_live_drivers.sh -- to tell "the release job needs the test job"
# from "the file contains the word needs somewhere". Shared rather than
# copied into each, so their readings of the same YAML cannot drift apart.
#
# A job block is one case of yaml_block above, and is spelled as one: the
# job name is what varies, and the nesting rule should not be restated per
# caller.
# <workflow-file> <job-name>
workflow_job_block() {
  yaml_block "$1" "  $2:"
}

# The names of every job in a workflow, one per line. <workflow-file>
workflow_job_names() {
  awk '
    /^jobs:/ { injobs = 1; next }
    injobs && /^  [^ #]/ && /:/ { sub(/:.*/, ""); gsub(/ /, ""); print }
  ' "$1"
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
