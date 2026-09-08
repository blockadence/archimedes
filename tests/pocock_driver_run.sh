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
# that writes nothing, a session that dies, and -- the one this driver
# exists to survive -- a session that ignores the prompt and writes all over
# the repo are ordinary outcomes of a headless agent run, and none of them
# is something you can ask a real, billed, minutes-long `claude -p` to do on
# demand.
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
write_map() {
  cat > CONTEXT.md <<'MAP'
# Widget Catalog

**Widget** -- a catalog entry with a name and a price in cents.
MAP
}
# The session doing what the prompt spent a paragraph asking it not to.
# Deliberately not just the ADR the prompt argues with: a settings file the
# skill thought was a favour, and an edit to a source file it thought was
# tidying, are the shapes a cleanup written as a list of known leftovers
# would walk straight past.
write_more_than_the_map() {
  mkdir -p docs/adr .claude
  echo "# 1. Widgets are priced in cents" > docs/adr/001-widgets.md
  echo '{"model":"opus"}' > .claude/settings.local.json
  echo "// tidied up while I was here" >> src/index.js
}
case "${CLAUDE_STUB_MODE:-write}" in
  write)
    write_map ;;
  noop)
    : ;;   # a session that read the repo and wrote nothing
  fail)
    echo "claude: session failed" >&2; exit 1 ;;
  extra)
    write_map; write_more_than_the_map ;;
  commit)
    write_map
    echo "# 1. Widgets are priced in cents" > ADR.md
    git add -A
    # The one identity in this suite still spelled out at the call site, and
    # deliberately not the fixture's: the case is a commit somebody else made
    # in the operator's repo, so it has to be somebody else's name on it.
    git -c user.email=agent@example.com -c user.name=agent commit -qm "agent committed its own work"
    ;;
  hang)
    # Everything a run that got all the way through would have done, the
    # disobedience included -- so an interrupt that gets swallowed rather
    # than acted on finishes normally and leaves both behind, which is
    # exactly the failure this mode exists to catch. Then it announces
    # itself and sits in the foreground until released, so the test can
    # signal the driver while bash is blocked on a child, which is where a
    # real Ctrl-C lands.
    write_map; write_more_than_the_map
    touch "$CLAUDE_STUB_SENTINEL"
    waited=0
    until [ -f "$CLAUDE_STUB_SENTINEL.release" ] || [ "$waited" -ge 300 ]; do
      sleep 0.1; waited=$((waited + 1))
    done
    ;;
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
assert_widget_repo_pristine "$REPO" "successful run"

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
assert_widget_repo_pristine "$REPO" "session wrote nothing"

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
assert_widget_repo_pristine "$REPO" "session failed"

echo ""
echo "pocock driver, the session writes more than the map:"

# The case the whole driver turns on. The prompt asks the session, at
# length, to write CONTEXT.md and nothing else; the session is a
# non-deterministic agent and the domain-modeling skill's own criteria call
# for ADRs, so the prompt is arguing with the thing it invokes and will
# sometimes lose. Losing must not look like winning.
fresh_repo "$REPO"
OUT="$WORK/extra.md"
if err="$(CLAUDE_STUB_MODE=extra "$ARCHIMEDES_BIN" run-driver pocock "$REPO" "$OUT" 2>&1 >/dev/null)"; then
  fail "a run whose session wrote more than the map exits non-zero"
else
  pass "a run whose session wrote more than the map exits non-zero"
fi
# Named, because the operator's next question is "what did it write?", and
# a run that says only "it wrote something" sends them looking through a
# repo this driver has already tidied.
assert_contains "$err" "docs/adr/001-widgets.md" "the failure names the file the session was told not to write"
assert_contains "$err" ".claude/settings.local.json" "and the one no prompt thought to forbid"
assert_contains "$err" "src/index.js" "and the tracked file it edited, which is not a leftover but a change"
assert_file_missing "$OUT" "no context map is harvested from a run that would not keep to one file"
assert_widget_repo_pristine "$REPO" "session wrote more than the map"
assert_eq "$(cat "$REPO/src/index.js" 2>/dev/null | tail -1)" "module.exports = { Widget };" \
  "the tracked file the session edited is put back to what HEAD says"

echo ""
echo "pocock driver, a repo that was already dirty:"

# The other half of "never worse off": putting the run's work back must not
# take the operator's with it. A repo with uncommitted work in it is the
# normal state of a repo somebody is working in, and the driver is pointed
# at those.
fresh_repo "$REPO"
echo "notes to self" > "$REPO/scratch-note.md"
echo "// mine, uncommitted" >> "$REPO/src/index.js"
before_status="$(git -C "$REPO" status --porcelain)"
OUT="$WORK/dirty.md"
if "$ARCHIMEDES_BIN" run-driver pocock "$REPO" "$OUT" >"$WORK/dirty.log" 2>&1; then
  pass "a run against a repo that was already dirty still succeeds"
else
  fail "a run against a repo that was already dirty still succeeds"
  cat "$WORK/dirty.log" >&2
fi
assert_file_exists "$REPO/scratch-note.md" "an untracked file that predates the run is still there"
assert_contains "$(cat "$REPO/src/index.js" 2>/dev/null)" "// mine, uncommitted" \
  "and an uncommitted edit to a tracked file is still there"
assert_eq "$(git -C "$REPO" status --porcelain)" "$before_status" \
  "the repo is dirty in exactly the way it was dirty before, and no other"

echo ""
echo "pocock driver, killed mid-run:"

# The case hand-placed error handling cannot reach, and the reason the
# rollback is the exit trap rather than something on the success path. By
# the moment of the kill the session has already written both the map and
# the files it was told not to, so a driver that shrugged the interrupt off
# would leave the lot in someone else's repo with nobody watching.
fresh_repo "$REPO"
SENTINEL="$WORK/session-started"
rm -f "$SENTINEL" "$SENTINEL.release"
# `set -m` gives the driver a process group of its own, so the interrupt can
# be delivered the way a real one is -- to the driver and its session
# together -- without taking this test process down with it.
set -m
CLAUDE_STUB_MODE=hang CLAUDE_STUB_SENTINEL="$SENTINEL" "$DRIVER_BIN" "$REPO" >"$WORK/killed.log" 2>&1 &
driver_pid=$!
set +m

waited=0
until [ -f "$SENTINEL" ] || [ "$waited" -ge 300 ]; do sleep 0.1; waited=$((waited + 1)); done

if [ -f "$SENTINEL" ]; then
  pass "the driver got as far as the session, with the session's writing done"
  assert_file_exists "$REPO/docs/adr/001-widgets.md" "what the session wrote really is in the repo at the moment of the kill"
  kill -INT -"$driver_pid" 2>/dev/null
  touch "$SENTINEL.release"
  wait "$driver_pid"; killed_status=$?
  if [ "$killed_status" -ne 0 ]; then
    pass "a driver interrupted mid-run exits non-zero rather than looking like a success"
  else
    fail "a driver interrupted mid-run exits non-zero rather than looking like a success"
  fi
  assert_widget_repo_pristine "$REPO" "killed mid-run"
  assert_file_missing "$REPO/CONTEXT.md" \
    "an interrupted run leaves nothing behind to harvest, the map included"
else
  fail "the driver got as far as the session, with the session's writing done (timed out waiting)"
  touch "$SENTINEL.release"
  kill -INT -"$driver_pid" 2>/dev/null
  wait "$driver_pid" 2>/dev/null
fi

echo ""
echo "pocock driver, the session commits:"

# Everything the cleanup reasons about is relative to the commit HEAD
# pointed at when the run started, so a session that commits has put the
# repo somewhere the driver cannot unwind -- while leaving `git status`
# reading clean, which is what makes quiet success here so bad a failure.
fresh_repo "$REPO"
OUT="$WORK/committed.md"
if err="$(CLAUDE_STUB_MODE=commit "$ARCHIMEDES_BIN" run-driver pocock "$REPO" "$OUT" 2>&1 >/dev/null)"; then
  fail "a run whose session committed exits non-zero rather than quietly succeeding"
else
  pass "a run whose session committed exits non-zero rather than quietly succeeding"
fi
assert_contains "$err" "HEAD moved" "the failure explains that the repo moved out from under the cleanup"
assert_contains "$err" "by hand" "the failure tells the operator the repo needs their attention"
assert_file_missing "$OUT" "no context map is harvested from a repo the driver could not clean up"

echo ""
echo "pocock driver, the target is not a git repo:"

# The cleanup is a diff against git, so a target with no git in it is a
# target this driver cannot promise anything about. Said before the session
# runs, because afterwards is after a billed call has already written into
# a directory nothing can put back.
NOT_A_REPO="$WORK/not-a-repo"
rm -rf "$NOT_A_REPO"; mkdir -p "$NOT_A_REPO/src"
if err="$("$DRIVER_BIN" "$NOT_A_REPO" 2>&1 >/dev/null)"; then
  fail "the driver refuses a target that is not a git repo"
else
  pass "the driver refuses a target that is not a git repo"
fi
assert_contains "$err" "not a git repo" "the refusal says what is wrong with the target"
assert_file_missing "$NOT_A_REPO/CONTEXT.md" "and refuses before any session writes into it"

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
