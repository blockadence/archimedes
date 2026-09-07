#!/usr/bin/env bash
# The one throwaway-repo fixture the bash tests share: a bare "origin" plus
# a clone of it carrying one commit, already pushed to main — the shape that
# makes a test's fetch/rev-parse/push work against real git with no network.
# The Go tests build the same shape with internal/testrepo.
#
# Sourcing this file defines one function and nothing else: it sets no shell
# options and declares no test-reporting helpers, so a test keeps its own
# `set -e` and its own pass/fail contract.

# make_origin_and_clone_at <origin-path> <clone-path> [seed-file] [seed-content]
#
# With no seed file the initial commit is empty. The clone gets a fixed
# identity and no commit signing, so committing here — now and in whatever
# the test commits on top — doesn't depend on the machine's git config.
make_origin_and_clone_at() {
  local origin="$1" clone="$2" seed="${3:-}" content="${4:-}"

  git init -q --bare -b main "$origin"
  git clone -q "$origin" "$clone"
  git -C "$clone" config user.email t@t
  git -C "$clone" config user.name t
  git -C "$clone" config commit.gpgsign false

  if [ -n "$seed" ]; then
    printf '%s' "$content" > "$clone/$seed"
    git -C "$clone" add -A
    git -C "$clone" commit -q -m init
  else
    git -C "$clone" commit -q --allow-empty -m init
  fi

  git -C "$clone" push -q -u origin main
}
