#!/bin/sh
# Spawn the humanizer MCP server inside its sandbox (anthropics/sandbox-runtime,
# launched through srt-run).
#
# Exists because servers.json can't express the whole launch:
#   - secret resolution has to happen outside the sandbox, which can't read
#     the secrets file
#   - srt doesn't filter the environment, so it is rebuilt with an env -i
#     allow-list and shell secrets never enter the confined process
#   - the MCP client spawns from the session cwd, which may sit inside a
#     read-denied subtree; getcwd() there is an EPERM, so cd / first
#   - the settings file names no host. The judge backend depends on which
#     secrets this machine has, so the wrapper derives the host and passes it
#     as SRT_RUN_ALLOW_DOMAINS, which srt-run merges into the allow-list at
#     launch
#
# The sandbox settings are ~/.config/srt/humanizer.json. This repo does not
# ship them: the consuming repo provides the file as part of its machine-private
# overlay (docs/adr/0006), next to the MCP registration that points here. The
# code roots the server may read differ per machine, which is why the manifest
# is the consuming repo's; examples/dotfiles/recipes/humanizer is one to start
# from. Without the file srt exits 1 and humanizer never starts (fail closed).
set -eu

# humanizer_judge needs an LLM backend, and MCP hosts launched outside a login
# shell don't carry the env. Pull only the backend vars from the ralph-managed
# secrets file (legacy ~/.secrets.sh as a fallback) in a subshell; never source
# the whole file into the sandboxed server's env. The personal profile provides
# OPENROUTER_API_KEY; the work profile provides LITELLM_BASE_URL, LITELLM_API_KEY
# and HUMANIZER_MODEL. humanizer selects the backend from whichever is set, so
# generic var names only: the proxy host and token stay in that gitignored file.
for _sf in "$HOME/.config/ralph/secrets.sh" "$HOME/.secrets.sh"; do
  [ -f "$_sf" ] || continue
  for _v in OPENROUTER_API_KEY LITELLM_BASE_URL LITELLM_API_KEY HUMANIZER_MODEL; do
    eval "_cur=\${$_v:-}"
    [ -n "$_cur" ] && continue
    # secrets file path is runtime-selected and read only in this subshell.
    # shellcheck disable=SC1090
    _val="$(set +eu; . "$_sf" >/dev/null 2>&1; eval "printf '%s' \"\${$_v:-}\"")"
    [ -n "$_val" ] && eval "export $_v=\"\$_val\""
  done
done

# The env allow-list: HOME, PATH, TMPDIR (srt-run hands it to the child), and
# the four backend vars when set.
set -- HOME="$HOME" PATH="${PATH:-/usr/bin:/bin}"
[ -n "${TMPDIR:-}" ] && set -- "$@" TMPDIR="$TMPDIR"
for _v in OPENROUTER_API_KEY LITELLM_BASE_URL LITELLM_API_KEY HUMANIZER_MODEL; do
  eval "_cur=\${$_v:-}"
  [ -n "$_cur" ] && set -- "$@" "$_v=$_cur"
done

# The judge host, picked the way humanizer picks its backend: a LiteLLM proxy
# wins, else OpenRouter. With neither set, no host is allowed and the judge
# reports itself unavailable.
_host=
if [ -n "${LITELLM_BASE_URL:-}" ]; then
  # https://user@proxy.example.com:4000/v1 -> proxy.example.com:4000
  _host=${LITELLM_BASE_URL#*://}
  _host=${_host%%/*}
  _host=${_host%%\?*}
  _host=${_host%%#*}
  _host=${_host##*@}
elif [ -n "${OPENROUTER_API_KEY:-}" ]; then
  _host=openrouter.ai
fi
[ -n "$_host" ] && set -- "$@" SRT_RUN_ALLOW_DOMAINS="$_host"
unset _sf _v _cur _val _host 2>/dev/null || true

cd /
exec /usr/bin/env -i "$@" \
  "$HOME/code/bin/srt-run" "$HOME/.config/srt/humanizer.json" \
  "$HOME/code/bin/humanizer" mcp
