#!/usr/bin/env bash
# Who owns a driver, proved through the binary an operator actually installs.
#
# The answer this suite pins down: the drivers archimedes ships live in the
# binary and are read out of it at the moment one runs, so fixing one here
# fixes it for instances that already exist -- upgrading the tool is the
# whole of the update route, and there is no step that copies tooling into
# an instance. A driver under an instance's own drivers/ is the instance's
# instead: it wins over a shipped one of the same name, and nothing ever
# refreshes or overwrites it.
#
# Everything here runs against a copy of the binary somewhere else on disk,
# with no checkout of this repo in reach, because "the binary carries it" is
# the claim being tested.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

build_archimedes || exit 1

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Scaffolding an instance makes a commit, so this file needs an identity.
# It gets one here rather than from the CI workflow -- see isolate_git in
# tests/gitfixture.sh for why that distinction matters.
isolate_git "$WORK"

INSTALLED="$WORK/bin/archimedes"
mkdir -p "$WORK/bin"
cp "$ARCHIMEDES_BIN" "$INSTALLED"

# A PATH holding bash and nothing else, so a driver that runs gets as far as
# its own dependency check and stops there. Reaching that error is the proof
# that matters: the manifest resolved, the command was found, and the
# command ran -- all of it out of the binary.
STUB_BIN="$WORK/stub-bin"
mkdir -p "$STUB_BIN"
for tool in bash git; do
  resolved="$(command -v "$tool" 2>/dev/null)" && ln -sf "$resolved" "$STUB_BIN/$tool"
done

"$INSTALLED" init widgets "$WORK" >/dev/null 2>&1 || {
  echo "could not scaffold an instance" >&2; exit 1
}
INSTANCE="$WORK/widgets"
REPO="$WORK/throwaway-repo"
make_widget_repo "$REPO"

echo "a driver the instance does not have still runs, out of the binary:"

assert_dir_missing "$INSTANCE/drivers/openspec" \
  "the instance holds no copy of openspec, so anything that runs came from the tool"

out_path="$WORK/out.md"
err="$(cd "$INSTANCE" && PATH="$STUB_BIN" "$INSTALLED" run-driver openspec "$REPO" "$out_path" 2>&1 >/dev/null)"
assert_contains "$err" "openspec CLI not found on PATH" \
  "the shipped driver's own dependency check is what fails, so its command ran out of the binary"
case "$err" in
  *"unknown driver"*) fail "a shipped driver resolves for an instance that carries no drivers of its own" ;;
  *) pass "a shipped driver resolves for an instance that carries no drivers of its own" ;;
esac
assert_file_missing "$out_path" "a failed run leaves no output file"

# The route by which a fix reaches an instance that already exists: there
# isn't one to run, because the instance never had a copy to be stale. The
# next `go install` is the delivery, and nothing under the instance changes.
assert_eq "$(git -C "$INSTANCE" status --porcelain)" "" \
  "running a shipped driver writes nothing into the instance -- no cache, no unpacked copy to go stale"

echo ""
echo "a driver the instance owns wins, and keeps winning:"

# Standing in for an operator who adopted openspec and edited it -- or for
# an instance created back when the drivers were copied in.
mkdir -p "$INSTANCE/drivers"
cp -R "$HERE/fixtures/drivers/stub-ok" "$INSTANCE/drivers/openspec"
sed -i.bak 's/^name: stub-ok/name: openspec/' "$INSTANCE/drivers/openspec/driver.yaml"
rm -f "$INSTANCE/drivers/openspec/driver.yaml.bak"

if (cd "$INSTANCE" && PATH="$STUB_BIN" "$INSTALLED" run-driver openspec "$REPO" "$out_path") >"$WORK/own.log" 2>&1; then
  pass "the instance's own driver runs where the shipped one would have"
else
  fail "the instance's own driver runs where the shipped one would have"
  cat "$WORK/own.log" >&2
fi
assert_contains "$(cat "$out_path" 2>/dev/null)" "stub-ok saw repo" \
  "the map is the instance's driver's output, not the shipped driver's"

echo ""
echo "the same name resolves the same driver, however it is reached:"

# `run-driver` and a context-mapping pass must never disagree about which
# driver a name means, whichever layer supplies it.
make_origin_and_clone "$WORK" mapped-repo
cat > "$INSTANCE/repos.yaml" <<EOF
driver: openspec
repos:
  - name: mapped-repo
    path: ../mapped-repo
    base_branch: main
    depends_on: []
    context_modeled_sha: null
EOF
(cd "$INSTANCE" && PATH="$STUB_BIN:$PATH" "$INSTALLED" context-map) </dev/null >"$WORK/pass.log" 2>&1
assert_contains "$(cat "$WORK/mapped-repo/CONTEXT.md" 2>/dev/null)" "stub-ok saw repo" \
  "a mapping pass resolves openspec to the instance's own driver too, exactly as run-driver did"

# And with the instance's copy gone, both routes fall back to the shipped
# one again -- which is what makes deleting it a real way to opt back in.
rm -rf "$INSTANCE/drivers/openspec"
err="$(cd "$INSTANCE" && PATH="$STUB_BIN" "$INSTALLED" run-driver openspec "$REPO" "$WORK/back.md" 2>&1 >/dev/null)"
assert_contains "$err" "openspec CLI not found on PATH" \
  "removing the instance's copy hands the name back to the driver the tool maintains"

echo ""
echo "an operator can see which is which, and take one over:"

listing="$(cd "$INSTANCE" && "$INSTALLED" drivers)"
assert_contains "$listing" "built-in" "the listing says the shipped drivers come from the tool"
for name in openspec pocock spec-kit; do
  assert_contains "$listing" "$name" "the listing names the $name driver"
done

adopted="$(cd "$INSTANCE" && "$INSTALLED" drivers adopt spec-kit)"
assert_contains "$adopted" "drivers/spec-kit" "adopting says where the driver landed"
assert_file_exists "$INSTANCE/drivers/spec-kit/run.sh" "adopting leaves the real driver to edit"
# The snapshot/restore helper is shared by the drivers that have to leave
# someone else's repo as they found it, so it sits beside them rather than
# inside one of them -- and an adopted driver that arrived without it would
# fail only once it was already running in a repository.
assert_file_exists "$INSTANCE/drivers/lib/repo-snapshot.sh" \
  "adopting brings the helpers the command sources, not just the command"
[ -x "$INSTANCE/drivers/spec-kit/run.sh" ] \
  && pass "the adopted driver's command arrives runnable" \
  || fail "the adopted driver's command arrives runnable"
[ -x "$INSTANCE/drivers/lib/repo-snapshot.sh" ] \
  && fail "a sourced helper is not a program and must not be made one" \
  || pass "a sourced helper is not a program and must not be made one"

listing="$(cd "$INSTANCE" && "$INSTALLED" drivers)"
assert_contains "$listing" "shadows built-in" \
  "the listing flags the adopted copy as one no fix to the shipped driver will reach"

# And the helpers the adoption brought are not themselves a driver: they
# declare no manifest, so lib/ must not appear as a name this instance can
# run -- nor as a broken one it cannot.
names="$(printf '%s\n' "$listing" | awk 'NR > 1 { print $1 }')"
assert_not_contains "$names" "lib" "the shared helper directory is not listed as a driver"

if err="$(cd "$INSTANCE" && "$INSTALLED" drivers adopt spec-kit 2>&1 >/dev/null)"; then
  fail "adopting over a driver the instance already has is refused"
else
  assert_contains "$err" "already exists" "adopting over a driver the instance already has is refused"
fi

report
