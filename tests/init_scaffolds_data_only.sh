#!/usr/bin/env bash
# A scaffolded instance is data and nothing else: a manifest, dossier and
# work directories, and the drivers/scaffolding it owns from there on. The
# tooling that acts on it is the globally installed `archimedes` binary, so
# nothing executable is copied in and there is nothing to re-vendor later.
#
# This is the guarantee that decays quietly: adding one convenience script
# back to template/ would go unnoticed until an instance somewhere had a
# stale copy of it.
#
# Scaffolding is itself the binary's job now (`archimedes init`), so this
# runs it the way a first-time user meets it: a copy of the binary on its
# own, with no checkout of this repo in reach.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"

build_archimedes || exit 1

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "a scaffolded instance holds data only:"

# The binary is copied out of the repo and run from elsewhere, so a pass
# here can't be coming from a checkout it found next to itself.
INSTALLED="$WORK/bin/archimedes"
mkdir -p "$WORK/bin"
cp "$ARCHIMEDES_BIN" "$INSTALLED"

if ( cd "$WORK" && "$INSTALLED" init widgets "$WORK" ) >"$WORK/init.log" 2>&1; then
  pass "the installed binary scaffolds an instance with no checkout in reach"
else
  fail "the installed binary scaffolds an instance with no checkout in reach"
  cat "$WORK/init.log" >&2
  report
  exit 1
fi

INSTANCE="$WORK/widgets"

assert_dir_missing "$INSTANCE/scripts" "no scripts/ directory is vendored into the instance"

# Drivers are the one thing an instance holds that happens to be
# executable, and they are its own configuration rather than a copy of
# tooling: which one maps a repo is a per-instance decision, and a driver is
# whatever program answers it. Everything else must be inert.
strays="$(cd "$INSTANCE" && find . -path ./.git -prune -o -type f \
  ! -path './drivers/*' \( -name '*.sh' -o -perm -u+x \) -print | sort | tr '\n' ' ')"
assert_eq "$strays" "" "nothing executable or shell-shaped is copied in outside drivers/"

for path in repos.yaml WORKSPACE-MAP.md AGENTS.md README.md; do
  assert_file_exists "$INSTANCE/$path" "the instance carries its $path"
done
for path in repos work drivers scaffolding convention-packs; do
  assert_dir_exists "$INSTANCE/$path" "the instance carries its $path/"
done

# A driver's command has to arrive runnable, and it is the manifest that
# says which file that is — nothing in the binary carries file modes.
[ -x "$INSTANCE/drivers/openspec/run.sh" ] \
  && pass "a scaffolded driver's command is still executable" \
  || fail "a scaffolded driver's command is still executable"

# The instance owns its drivers and scaffolding from here, so there is no
# refresh-from-Archimedes step in either direction. Asserted as the absence
# of the thing itself rather than of the words: the README names the retired
# script on purpose, telling operators of older instances what became of it.
revendor="$(find "$ROOT" -name .git -prune -o -name 'update-from-archimedes*' -print | tr '\n' ' ')"
assert_eq "$revendor" "" "the re-vendoring script is gone from the repo"
assert_file_missing "$INSTANCE/scripts/update-from-archimedes.sh" \
  "and a fresh instance is not handed one"

# Scaffolding from a checkout was the same job done a second way, and a
# second way drifts. There is one now.
assert_file_missing "$ROOT/scripts/init.sh" "scaffolding from a checkout is gone from the repo"

commits="$(git -C "$INSTANCE" rev-list --count HEAD)"
assert_eq "$commits" "1" "the instance starts on its own fresh history"

# Refusing an occupied destination is what keeps a mistyped name from
# merging a template into somebody's existing directory.
mkdir -p "$WORK/taken"
if err="$( "$INSTALLED" init taken "$WORK" 2>&1 >/dev/null )"; then
  fail "init refuses a destination that already exists"
else
  assert_contains "$err" "already exists" "init refuses a destination that already exists"
fi

report
