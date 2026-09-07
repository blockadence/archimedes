#!/usr/bin/env bash
# What the suite runner says about the files it did not run.
#
# Every route into this suite -- a developer before pushing, the CI job that
# gates a release -- reads one line to decide whether the tree is good. That
# line has to distinguish "nine files passed" from "seven passed and two
# never ran", because the two opt-in e2e files skip on any machine without
# a `claude` CLI and a live-driver opt-in, which is every CI runner we have.
# A runner that prints "0 failed" for both states is the exact confidence
# this repo hasn't earned.
#
# So the runner is driven here against a directory of stand-in test files
# whose outcomes are fixed by construction -- one that passes, one that
# skips, one that fails -- rather than against the real suite, whose skips
# depend on what happens to be installed on the machine running it.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

RUN_ALL="$HERE/run-all.sh"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# <dir> <name> <exit-status> -- a stand-in test file that announces itself
# and exits with the status the runner has to interpret.
make_stub() {
  local dir="$1" name="$2" status="$3"
  mkdir -p "$dir"
  cat > "$dir/$name" <<EOF
#!/usr/bin/env bash
echo "ran $name"
exit $status
EOF
  chmod +x "$dir/$name"
}

echo "the suite runner accounts for the files it did not run:"

# 77 is the skip status `make check` has used for decades; the alternative
# considered was parsing a "skip:" line out of stdout, which makes any test
# that merely mentions the word skip a skip.
GREEN="$WORK/green"
make_stub "$GREEN" "a_passes.sh" 0
make_stub "$GREEN" "b_skips.sh" 77

out="$("$RUN_ALL" "$GREEN" 2>&1)"; status=$?
assert_eq "$status" "0" "a suite of passes and skips exits zero"
assert_contains "$out" "1 skipped" "the summary counts the skipped file"
assert_contains "$out" "skipped: b_skips.sh" "the summary names the skipped file"
assert_not_contains "$out" "skipped: a_passes.sh" "a file that ran is not listed among the skipped"

# The reason this file exists: the two states must not read alike.
ALL_RAN="$WORK/all-ran"
make_stub "$ALL_RAN" "a_passes.sh" 0
make_stub "$ALL_RAN" "b_also_passes.sh" 0

allout="$("$RUN_ALL" "$ALL_RAN" 2>&1)"
assert_contains "$allout" "0 skipped" "a fully-run suite says so"
[ "$out" != "$allout" ] \
  && pass "a suite with a skipped file does not read like one without" \
  || fail "a suite with a skipped file does not read like one without"

# A skip is not a pass: it must not mask a failure anywhere in the run.
RED="$WORK/red"
make_stub "$RED" "a_passes.sh" 0
make_stub "$RED" "b_skips.sh" 77
make_stub "$RED" "c_fails.sh" 1

redout="$("$RUN_ALL" "$RED" 2>&1)"; redstatus=$?
assert_eq "$redstatus" "1" "one failing file fails the run even alongside a skip"
assert_contains "$redout" "failed: c_fails.sh" "the summary names the failing file"

# The runner's own scaffolding is not a test. Left in, helpers.sh would run
# as one every time and gitfixture.sh would too.
SCAFFOLD="$WORK/scaffold"
make_stub "$SCAFFOLD" "a_passes.sh" 0
make_stub "$SCAFFOLD" "helpers.sh" 1
make_stub "$SCAFFOLD" "gitfixture.sh" 1
make_stub "$SCAFFOLD" "run-all.sh" 1

scaffoldout="$("$RUN_ALL" "$SCAFFOLD" 2>&1)"; scaffoldstatus=$?
assert_eq "$scaffoldstatus" "0" "the shared scaffolding is not run as a test"
assert_not_contains "$scaffoldout" "ran helpers.sh" "helpers.sh is not run as a test"

# Every file's own output still reaches the reader; the summary is added to
# what was already there, not a replacement for it.
assert_contains "$out" "ran a_passes.sh" "each file's own output is still shown"

# The summary is built from file names, which are the one input here nobody
# controls: a glob character or a space in one must reach the reader as
# itself rather than being expanded or split.
ODD="$WORK/odd-names"
make_stub "$ODD" "a b.sh" 77
make_stub "$ODD" "c[0-9].sh" 77

oddout="$("$RUN_ALL" "$ODD" 2>&1)"
assert_contains "$oddout" "skipped: a b.sh" "a file name with a space is reported whole"
assert_contains "$oddout" "skipped: c[0-9].sh" "a file name that looks like a glob is reported literally"

# A directory with nothing in it is an empty run, not a failing one: the
# glob stays literal when it matches nothing, and running it would report
# a bad path as a broken test.
EMPTY="$WORK/empty"
mkdir -p "$EMPTY"
emptyout="$("$RUN_ALL" "$EMPTY" 2>&1)"; emptystatus=$?
assert_eq "$emptystatus" "0" "a directory with no test files is not a failure"
assert_contains "$emptyout" "0 files:" "an empty run says it ran nothing"

report
