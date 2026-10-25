#!/bin/bash
# Exercises the speak launchers against a scratch HOME with stand-ins for
# speak and srt-run that record what they were handed and run nothing.
# speak-serve.sh (the speak-web agent) must hand srt-run the speak-web
# settings, the provider's host as SRT_RUN_ALLOW_DOMAINS, and only the
# variables the provider reads; speak-env.sh (the MCP server) must exec speak
# directly with the same variables. Bash 3.2 compatible.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVE="$ROOT/recipes/speak/speak-serve.sh"
ENV_WRAPPER="$ROOT/recipes/speak/speak-env.sh"

scratch=$(mktemp -d "${TMPDIR:-/tmp}/speak-serve-test.XXXXXX")
trap 'rm -r "$scratch"' EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

# new_home <name>: a fresh HOME with a secrets file, a stand-in srt-run at
# ~/code/bin/srt-run and a stand-in speak. The stand-in speak answers
# `config env` with $STANDIN_ENV and `config host` with $STANDIN_HOST; any
# other call, like srt-run, records its argv, cwd and environment. Sets $home.
new_home() {
  home="$scratch/$1"
  mkdir -p "$home/code/bin" "$home/.config/ralph"
  cat > "$home/.config/ralph/secrets.sh" << 'SECRETS'
export OPENROUTER_API_KEY=sk-from-secrets
export UNRELATED_VAR=stays-in-the-file
SECRETS
  cat > "$home/code/bin/srt-run" << 'STANDIN'
#!/bin/sh
printf '%s\n' "$@" > "$HOME/srt-run.args"
pwd > "$HOME/srt-run.cwd"
env > "$HOME/srt-run.env"
STANDIN
  cat > "$home/code/bin/speak" << 'STANDIN'
#!/bin/sh
case "$1 ${2:-}" in
  "config env") [ -z "${STANDIN_ENV:-}" ] || printf '%s\n' $STANDIN_ENV; exit 0 ;;
  "config host") [ -z "${STANDIN_HOST:-}" ] || printf '%s\n' "$STANDIN_HOST"; exit 0 ;;
esac
printf '%s\n' "$@" > "$HOME/speak.args"
env > "$HOME/speak.env"
STANDIN
  chmod +x "$home/code/bin/srt-run" "$home/code/bin/speak"
}

# run <script> [VAR=value...] -- <args...>: run a launcher under $home with a
# clean environment.
run() {
  local script=$1
  shift
  local -a vars=()
  while [ "$1" != -- ]; do
    vars+=("$1")
    shift
  done
  shift
  env -i HOME="$home" PATH=/usr/bin:/bin ${vars[@]+"${vars[@]}"} sh "$script" "$@"
}

has_env() { grep -qx "$1" "$home/$2.env"; }
lacks_var() { ! grep -q "^$1=" "$home/$2.env"; }

# A remote provider: srt-run gets the settings, the host, the key, and the
# serve command; nothing else from the secrets file crosses.
new_home remote
run "$SERVE" STANDIN_ENV=OPENROUTER_API_KEY STANDIN_HOST=openrouter.ai -- --port 7425 ||
  fail "remote launch exited $?"
expected_args="$home/.config/srt/speak-web.json
$home/code/bin/speak
serve
--port
7425"
[ "$(cat "$home/srt-run.args")" = "$expected_args" ] ||
  fail "srt-run args: $(tr '\n' ' ' < "$home/srt-run.args")"
[ "$(cat "$home/srt-run.cwd")" = / ] || fail "srt-run cwd: $(cat "$home/srt-run.cwd")"
has_env SRT_RUN_ALLOW_DOMAINS=openrouter.ai srt-run || fail "no SRT_RUN_ALLOW_DOMAINS for the host"
has_env OPENROUTER_API_KEY=sk-from-secrets srt-run || fail "the provider key did not reach srt-run"
lacks_var UNRELATED_VAR srt-run || fail "an unrelated secret reached srt-run"

# A host with a port, appended to a caller's own entry.
new_home port
run "$SERVE" STANDIN_HOST=llm.example.com:4000 SRT_RUN_ALLOW_DOMAINS=other.example.org -- ||
  fail "port launch exited $?"
has_env SRT_RUN_ALLOW_DOMAINS=other.example.org,llm.example.com:4000 srt-run ||
  fail "host was not appended: $(grep SRT_RUN_ALLOW_DOMAINS "$home/srt-run.env")"

# The local engine: no host, no key, so no egress beyond loopback.
new_home local
run "$SERVE" -- --port 7425 || fail "local launch exited $?"
lacks_var SRT_RUN_ALLOW_DOMAINS srt-run || fail "local provider set SRT_RUN_ALLOW_DOMAINS"
lacks_var OPENROUTER_API_KEY srt-run || fail "local provider got a key"

# A key already in the environment wins over the secrets file.
new_home preset
run "$SERVE" STANDIN_ENV=OPENROUTER_API_KEY OPENROUTER_API_KEY=sk-preset -- ||
  fail "preset launch exited $?"
has_env OPENROUTER_API_KEY=sk-preset srt-run || fail "the preset key was overridden"

# speak-env.sh stays unsandboxed: speak itself runs, with the same key.
new_home mcp
run "$ENV_WRAPPER" STANDIN_ENV=OPENROUTER_API_KEY STANDIN_HOST=openrouter.ai -- mcp ||
  fail "speak-env.sh exited $?"
[ ! -e "$home/srt-run.args" ] || fail "speak-env.sh went through srt-run"
[ "$(cat "$home/speak.args")" = mcp ] || fail "speak args: $(cat "$home/speak.args")"
has_env OPENROUTER_API_KEY=sk-from-secrets speak || fail "the key did not reach speak mcp"
lacks_var UNRELATED_VAR speak || fail "an unrelated secret reached speak mcp"
lacks_var SRT_RUN_ALLOW_DOMAINS speak || fail "speak-env.sh set SRT_RUN_ALLOW_DOMAINS"

printf 'PASS: speak launchers (serve under srt-run, mcp direct)\n'
