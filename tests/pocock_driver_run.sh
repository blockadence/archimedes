#!/usr/bin/env bash
# The pocock driver's own orchestration -- run the session inside the
# target repo, honour the fixed-location contract, refuse to report success
# when nothing was written -- exercised against a stub `claude` CLI. No
# network, no billed call, so this runs in the normal suite, the same way
# spec_kit_driver_run.sh covers the spec-kit driver.
#
# What this file does not claim, and must not be read as claiming: that the
# domain-modeling skill still produces a usable context map. Every
# assertion below is satisfied by a stub that writes four words to
# CONTEXT.md, so what is proved here is that *our* half still holds -- the
# half most likely to break from our own edits. The upstream half is
# pocock-driver-e2e.sh, which makes the real billed call and is run by
# .github/workflows/live-drivers.yml on a schedule.
#
# The stubs are what make the failure paths reachable at all: a session
# that writes nothing and a session that dies are ordinary outcomes of a
# headless agent run, and neither is something you can ask a real, billed,
# minutes-long `claude -p` to do on demand.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
DRIVER_BIN="$ROOT/drivers/pocock/run.sh"
build_archimedes || exit 1

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

STUB_BIN="$WORK/bin"
mkdir -p "$STUB_BIN"

# Stands in for the headless `claude -p` session. Records where it was run
# and what it was asked to do -- the driver's whole job is getting those two
# right -- then behaves the way CLAUDE_STUB_MODE says.
cat > "$STUB_BIN/claude" <<'STUB'
#!/usr/bin/env bash
set -uo pipefail
{
  echo "cwd=$PWD"
  for a in "$@"; do echo "arg=$a"; done
} > "$CLAUDE_STUB_LOG"
case "${CLAUDE_STUB_MODE:-write}" in
  write)
    cat > CONTEXT.md <<'MAP'
# Widget Catalog

**Widget** -- a catalog entry with a name and a price in cents.
MAP
    ;;
  noop)
    : ;;   # a session that read the repo and wrote nothing
  fail)
    echo "claude: session failed" >&2; exit 1 ;;
esac
exit 0
STUB
chmod +x "$STUB_BIN/claude"

export PATH="$STUB_BIN:$PATH"
export CLAUDE_STUB_LOG="$WORK/session.log"

REPO="$WORK/repo"

# <path> -- the same signature as spec_kit_driver_run.sh's, so the two
# don't read alike and mean different things. Clearing the session log is
# this file's addition: every case asks what the driver did to the stub,
# and a stale log would answer for the previous one.
fresh_repo() {
  rm -rf "$1"
  make_widget_repo "$1"
  rm -f "$CLAUDE_STUB_LOG"
}

# Nothing but .git and src/ ever belonged in this repo, so anything else is
# a trace of the run -- including the empty directories `git status` cannot
# see, which is why the neighbour checks this way too.
assert_repo_pristine() { # <repo> <label>
  local repo="$1" label="$2" leftovers
  leftovers="$(cd "$repo" && ls -A | sort | tr '\n' ' ')"
  assert_eq "$leftovers" ".git src " "$label: nothing is left in the repo but what it started with"
  assert_eq "$(git -C "$repo" status --porcelain)" "" "$label: the repo's git status is clean"
}

echo "pocock driver, successful run:"

fresh_repo "$REPO"
OUT="$WORK/CONTEXT.md"
if "$ARCHIMEDES_BIN" run-driver pocock "$REPO" "$OUT" >"$WORK/run.log" 2>&1; then
  pass "a run whose session writes CONTEXT.md exits zero"
else
  fail "a run whose session writes CONTEXT.md exits zero"
  cat "$WORK/run.log" >&2
fi
assert_file_exists "$OUT" "the context map is harvested to the exact requested path"
assert_contains "$(cat "$OUT" 2>/dev/null)" "Widget Catalog" \
  "the harvested file is what the session wrote"
assert_file_missing "$REPO/CONTEXT.md" \
  "CONTEXT.md is gone from the target repo, not just untracked"
assert_repo_pristine "$REPO" "successful run"

echo ""
echo "pocock driver, how the session is invoked:"

# The fixed-location contract rests entirely on this: the skill writes
# CONTEXT.md at the root of whatever repo it is invoked in and cannot be
# pointed anywhere else, so a driver that ran the session from its own
# working directory would harvest nothing -- or, worse, harvest this repo's
# CONTEXT.md.
session="$(cat "$CLAUDE_STUB_LOG" 2>/dev/null)"
assert_contains "$session" "cwd=$REPO" "the session runs inside the target repo, which is where the skill writes"
assert_contains "$session" "arg=-p" "the session is headless -- nobody is there to answer a prompt"
assert_contains "$session" "domain-modeling" "the session is told to use the domain-modeling skill"
assert_contains "$session" "CONTEXT.md" "the session is told which file to produce"
# The driver runner harvests exactly one file by moving it. Anything else
# the session writes is left in someone else's repo, so the prompt has to
# ask for that restraint even though nothing downstream can enforce it.
assert_contains "$session" "do not create or modify any other file" \
  "the session is told to write nothing but that file"

echo ""
echo "pocock driver, the session writes nothing:"

fresh_repo "$REPO"
OUT="$WORK/noop.md"
if err="$(CLAUDE_STUB_MODE=noop "$ARCHIMEDES_BIN" run-driver pocock "$REPO" "$OUT" 2>&1 >/dev/null)"; then
  fail "a run whose session wrote no CONTEXT.md exits non-zero"
else
  pass "a run whose session wrote no CONTEXT.md exits non-zero"
fi
assert_contains "$err" "did not produce" \
  "the failure says the session produced nothing, rather than blaming the driver runner"
assert_file_missing "$OUT" "a session that wrote nothing produces no output file"
assert_repo_pristine "$REPO" "session wrote nothing"

echo ""
echo "pocock driver, the session fails:"

fresh_repo "$REPO"
OUT="$WORK/fail.md"
if CLAUDE_STUB_MODE=fail "$ARCHIMEDES_BIN" run-driver pocock "$REPO" "$OUT" >/dev/null 2>&1; then
  fail "a run whose session exits non-zero fails the driver too"
else
  pass "a run whose session exits non-zero fails the driver too"
fi
assert_file_missing "$OUT" "a failed session produces no output file"
assert_repo_pristine "$REPO" "session failed"

echo ""
echo "pocock driver, claude is not installed:"

# The driver is a wrapper around one CLI. Someone who has not installed it
# should be told that, not handed a bare command-not-found from inside a
# subshell.
fresh_repo "$REPO"
# Run through this shell by its absolute path rather than the shebang:
# with nothing on PATH, `#!/usr/bin/env bash` cannot find bash either, and
# the test would pass on the wrong error.
mkdir -p "$WORK/empty"
if err="$(PATH="$WORK/empty" "$BASH" "$DRIVER_BIN" "$REPO" 2>&1 >/dev/null)"; then
  fail "the driver refuses to run without the claude CLI"
else
  pass "the driver refuses to run without the claude CLI"
fi
assert_contains "$err" "claude CLI not found" "the refusal names the missing CLI"

echo ""
echo "pocock driver, called wrongly:"

if err="$("$DRIVER_BIN" 2>&1 >/dev/null)"; then
  fail "the driver rejects a call with no repo path"
else
  pass "the driver rejects a call with no repo path"
fi
assert_contains "$err" "usage:" "the rejection says how to call it"

report
