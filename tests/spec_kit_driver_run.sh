#!/usr/bin/env bash
# The spec-kit driver's own orchestration -- snapshot, scaffold, check the
# constitution actually got filled in, restore keeping only that one file --
# exercised against stub `specify` and `claude` CLIs. No network, no billed
# calls, so this runs in the normal suite; spec-kit-driver-e2e.sh covers the
# same shape against the real tools and is opt-in.
#
# The stubs matter more than they look: they're what makes the failure paths
# testable at all. A driver that unpacks a toolchain into someone else's
# repo has to put it back on *every* exit -- the tool failing, the agent
# doing nothing, someone hitting Ctrl-C halfway -- and none of those are
# reachable when the run costs money and takes minutes.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
DRIVER_BIN="$ROOT/drivers/spec-kit/run.sh"
build_archimedes || exit 1

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

STUB_BIN="$WORK/bin"
mkdir -p "$STUB_BIN"

# Stands in for `specify init --here ...`: unpacks a miniature of the real
# layout -- a placeholder constitution, helper scripts, agent skills, and an
# empty directory -- into whatever repo it's run in.
cat > "$STUB_BIN/specify" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [ "${SPECIFY_STUB_FAIL:-0}" = "1" ]; then
  echo "specify: could not download templates" >&2
  exit 1
fi
mkdir -p .specify/memory .specify/scripts/bash .specify/workflows/speckit .claude/skills/speckit-constitution
cat > .specify/memory/constitution.md <<'TEMPLATE'
# [PROJECT_NAME] Constitution

### [PRINCIPLE_1_NAME]
[PRINCIPLE_1_DESCRIPTION]
TEMPLATE
echo "resolver" > .specify/scripts/bash/resolve-template.sh
echo "feature.json" > .specify/.gitignore
echo "skill" > .claude/skills/speckit-constitution/SKILL.md
if [ "${SPECIFY_STUB_NO_CONSTITUTION:-0}" = "1" ]; then
  rm .specify/memory/constitution.md
fi
exit 0
STUB

# Stands in for the headless `claude -p` session. CLAUDE_STUB_MODE picks
# which way the session goes.
cat > "$STUB_BIN/claude" <<'STUB'
#!/usr/bin/env bash
set -uo pipefail
case "${CLAUDE_STUB_MODE:-fill}" in
  fill)
    cat > .specify/memory/constitution.md <<'FILLED'
# Widget Catalog Constitution

### I. Zero-Dependency CommonJS Core
Every module stays dependency-free.
FILLED
    ;;
  noop)
    : ;;   # leaves the template exactly as specify unpacked it
  fail)
    echo "claude: session failed" >&2; exit 1 ;;
  commit)
    git add -A
    # Deliberately not the fixture's identity, as in pocock_driver_run.sh:
    # the case is a commit somebody else made in the operator's repo.
    git -c user.email=agent@example.com -c user.name=agent commit -qm "agent committed the scaffolding"
    echo "# filled" > .specify/memory/constitution.md
    ;;
  hang)
    # Does everything a successful session would -- so if the interrupt
    # gets swallowed rather than acted on, the run finishes normally and
    # reports success, which is exactly the failure being tested for.
    # Then announces itself and sits in the foreground until released, so
    # the test can signal the driver while bash is blocked on a child,
    # which is where a real Ctrl-C lands.
    cat > .specify/memory/constitution.md <<'FILLED'
# Widget Catalog Constitution

### I. Zero-Dependency CommonJS Core
Every module stays dependency-free.
FILLED
    touch "$CLAUDE_STUB_SENTINEL"
    waited=0
    until [ -f "$CLAUDE_STUB_SENTINEL.release" ] || [ "$waited" -ge 300 ]; do
      sleep 0.1; waited=$((waited + 1))
    done
    ;;
esac
exit 0
STUB
chmod +x "$STUB_BIN/specify" "$STUB_BIN/claude"

export PATH="$STUB_BIN:$PATH"

# Every case below starts from the same untouched repo and ends with the
# same question, asked by helpers.sh's assert_widget_repo_pristine: is it
# back exactly as it was found?
fresh_repo() { # <path>
  rm -rf "$1"
  make_widget_repo "$1"
}

REPO="$WORK/repo"

echo "spec-kit driver, successful run:"

fresh_repo "$REPO"
OUT="$WORK/CONTEXT.md"
if "$ARCHIMEDES_BIN" run-driver spec-kit "$REPO" "$OUT" >"$WORK/run.log" 2>&1; then
  pass "a run whose session fills the constitution in exits zero"
else
  fail "a run whose session fills the constitution in exits zero"
  cat "$WORK/run.log" >&2
fi
assert_file_exists "$OUT" "the constitution is harvested to the exact requested path"
assert_contains "$(cat "$OUT" 2>/dev/null)" "Zero-Dependency" \
  "the harvested file is what the session wrote, not the template specify unpacked"
assert_widget_repo_pristine "$REPO" "successful run"
assert_dir_missing "$REPO/.specify" "successful run: the scaffolding directory tree is gone, empty ones included"
assert_dir_missing "$REPO/.claude" "successful run: the installed agent skills are gone"

echo ""
echo "spec-kit driver, the session does nothing:"

fresh_repo "$REPO"
OUT="$WORK/noop.md"
if err="$(CLAUDE_STUB_MODE=noop "$ARCHIMEDES_BIN" run-driver spec-kit "$REPO" "$OUT" 2>&1 >/dev/null)"; then
  fail "a run whose session leaves the template untouched exits non-zero"
else
  pass "a run whose session leaves the template untouched exits non-zero"
fi
assert_contains "$err" "unfilled template" \
  "the failure says the session did nothing, rather than reporting a constitution that is really just placeholders"
assert_file_missing "$OUT" "a session that did nothing produces no output file"
assert_widget_repo_pristine "$REPO" "session did nothing"

echo ""
echo "spec-kit driver, the session fails:"

fresh_repo "$REPO"
OUT="$WORK/fail.md"
if CLAUDE_STUB_MODE=fail "$ARCHIMEDES_BIN" run-driver spec-kit "$REPO" "$OUT" >/dev/null 2>&1; then
  fail "a run whose session exits non-zero fails the driver too"
else
  pass "a run whose session exits non-zero fails the driver too"
fi
assert_file_missing "$OUT" "a failed session produces no output file"
assert_widget_repo_pristine "$REPO" "session failed"

echo ""
echo "spec-kit driver, specify itself fails:"

fresh_repo "$REPO"
OUT="$WORK/specify-fail.md"
if err="$(SPECIFY_STUB_FAIL=1 "$ARCHIMEDES_BIN" run-driver spec-kit "$REPO" "$OUT" 2>&1 >/dev/null)"; then
  fail "a run whose specify init fails exits non-zero"
else
  pass "a run whose specify init fails exits non-zero"
fi
assert_contains "$err" "specify init failed" "the failure names the step that failed"
assert_file_missing "$OUT" "a failed specify init produces no output file"
assert_widget_repo_pristine "$REPO" "specify init failed"

echo ""
echo "spec-kit driver, specify scaffolds no constitution:"

fresh_repo "$REPO"
OUT="$WORK/no-constitution.md"
if err="$(SPECIFY_STUB_NO_CONSTITUTION=1 "$ARCHIMEDES_BIN" run-driver spec-kit "$REPO" "$OUT" 2>&1 >/dev/null)"; then
  fail "a run where specify scaffolds no constitution exits non-zero"
else
  pass "a run where specify scaffolds no constitution exits non-zero"
fi
assert_contains "$err" "did not scaffold" "the failure names what specify was expected to produce"
assert_file_missing "$OUT" "no output file is produced"
assert_widget_repo_pristine "$REPO" "specify scaffolded no constitution"

echo ""
echo "spec-kit driver, killed mid-run:"

# The case hand-placed error handling can't reach, and the reason rollback
# is the exit trap rather than something on the success path.
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
  pass "the driver got as far as the session, with the scaffolding unpacked"
  assert_dir_exists "$REPO/.specify" "the scaffolding really is in the repo at the moment of the kill"
  kill -INT -"$driver_pid" 2>/dev/null
  touch "$SENTINEL.release"
  wait "$driver_pid"; killed_status=$?
  if [ "$killed_status" -ne 0 ]; then
    pass "a driver interrupted mid-run exits non-zero rather than looking like a success"
  else
    fail "a driver interrupted mid-run exits non-zero rather than looking like a success"
  fi
  # The session had already written the constitution by this point, so a
  # driver that shrugged the interrupt off would have left it sitting there
  # for the driver runner to harvest -- a context map for a repo nobody
  # finished cleaning up.
  assert_widget_repo_pristine "$REPO" "killed mid-run"
  assert_file_missing "$REPO/.specify/memory/constitution.md" \
    "an interrupted run leaves nothing behind to harvest, session's work included"
else
  fail "the driver got as far as the session, with the scaffolding unpacked (timed out waiting)"
  touch "$SENTINEL.release"
  kill -INT -"$driver_pid" 2>/dev/null
  wait "$driver_pid" 2>/dev/null
fi

echo ""
echo "spec-kit driver, the session commits:"

# Restore reasons entirely relative to the commit HEAD pointed at when the
# snapshot was taken, so a session that commits has put the repo somewhere
# the driver can't unwind. What matters is that it says so: `git status`
# reads clean either way, so a quiet success here would report a context map
# for a repo that now has a whole toolchain committed into it.
fresh_repo "$REPO"
OUT="$WORK/committed.md"
if err="$(CLAUDE_STUB_MODE=commit "$ARCHIMEDES_BIN" run-driver spec-kit "$REPO" "$OUT" 2>&1 >/dev/null)"; then
  fail "a run whose session committed exits non-zero rather than quietly succeeding"
else
  pass "a run whose session committed exits non-zero rather than quietly succeeding"
fi
assert_contains "$err" "HEAD moved" "the failure explains that the repo moved out from under the rollback"
assert_contains "$err" "by hand" "the failure tells the operator the repo needs their attention"
assert_file_missing "$OUT" "no context map is harvested from a repo the driver could not clean up"

report
