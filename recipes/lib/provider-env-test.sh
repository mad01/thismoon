#!/bin/bash
# Exercises export_provider_env from provider-env.sh against a scratch HOME.
# Only the named variables may leave the secrets file, and a variable already
# set wins. The ralph-managed file wins over the legacy ~/.secrets.sh, and a
# bad name fails before any secrets file is read. Bash 3.2 compatible; the
# helper itself runs under /bin/sh, as the launchers run it.
set -euo pipefail

HELPER="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/provider-env.sh"

scratch=$(mktemp -d "${TMPDIR:-/tmp}/provider-env-test.XXXXXX")
trap 'rm -r "$scratch"' EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

# new_home <name>: a fresh HOME whose ralph secrets file sets two provider
# variables and an unrelated one, and whose legacy file sets a third provider
# variable plus a different value for one of the first two. Sourcing either
# file leaves a marker, so a test can tell whether it was read. Sets $home.
new_home() {
  home="$scratch/$1"
  mkdir -p "$home/.config/ralph"
  cat > "$home/.config/ralph/secrets.sh" << 'SECRETS'
touch "$HOME/read.ralph"
export OPENROUTER_API_KEY=sk-ralph
export LITELLM_BASE_URL=https://llm.example.com
export UNRELATED_VAR=stays-in-the-file
SECRETS
  cat > "$home/.secrets.sh" << 'SECRETS'
touch "$HOME/read.legacy"
export OPENROUTER_API_KEY=sk-legacy
export LITELLM_API_KEY=sk-legacy-litellm # suspenders:ignore
SECRETS
}

# probe [VAR=value...] -- <names...>: source the helper under $home with a
# clean environment and set -eu (as the launchers run it), call it with the
# names, and write the resulting environment to $home/env.
probe() {
  local -a vars=()
  while [ "$1" != -- ]; do
    vars+=("$1")
    shift
  done
  shift
  # The script expands its own arguments, inside the clean sh.
  # shellcheck disable=SC2016
  env -i HOME="$home" PATH=/usr/bin:/bin ${vars[@]+"${vars[@]}"} \
    sh -c 'set -eu; . "$1"; shift; export_provider_env "$@"; env > "$HOME/env"' sh "$HELPER" "$@"
}

has_env() { grep -qx "$1" "$home/env"; }
lacks_var() { ! grep -q "^$1=" "$home/env"; }

# Each named variable comes from the first file that sets it; nothing else
# crosses.
new_home pull
probe -- OPENROUTER_API_KEY LITELLM_BASE_URL LITELLM_API_KEY || fail "pull exited $?"
has_env OPENROUTER_API_KEY=sk-ralph || fail "the ralph file did not win over the legacy one"
has_env LITELLM_BASE_URL=https://llm.example.com || fail "LITELLM_BASE_URL did not land"
has_env LITELLM_API_KEY=sk-legacy-litellm || fail "the legacy fallback did not land" # suspenders:ignore
lacks_var UNRELATED_VAR || fail "an unrelated secret crossed"

# A variable already in the environment wins; a name no file sets stays unset.
new_home preset
probe OPENROUTER_API_KEY=sk-preset -- OPENROUTER_API_KEY HUMANIZER_MODEL || fail "preset exited $?"
has_env OPENROUTER_API_KEY=sk-preset || fail "the preset key was overridden"
lacks_var HUMANIZER_MODEL || fail "a name no file sets came out set"

# No names: no secrets file is read.
new_home none
probe -- || fail "no-name call exited $?"
[ ! -e "$home/read.ralph" ] || fail "a no-name call read the secrets file"

# No secrets files at all is not an error.
new_home missing
rm "$home/.config/ralph/secrets.sh" "$home/.secrets.sh"
probe -- OPENROUTER_API_KEY || fail "missing files exited $?"
lacks_var OPENROUTER_API_KEY || fail "a key appeared with no secrets file"

# A name that is not a variable name fails before either file is read. The
# name carries a command that must never run.
new_home bad
# shellcheck disable=SC2016
if probe -- OPENROUTER_API_KEY 'X;touch $HOME/pwned' 2> "$home/stderr"; then
  fail "a bad name was accepted"
fi
[ ! -e "$home/read.ralph" ] || fail "the secrets file was read before the bad name failed"
[ ! -e "$home/pwned" ] || fail "a bad name was evaluated"
grep -q 'not a variable name' "$home/stderr" || fail "no error for the bad name: $(cat "$home/stderr")"

printf 'PASS: provider-env helper\n'
