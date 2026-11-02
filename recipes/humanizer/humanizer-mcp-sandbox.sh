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
# secrets file (legacy ~/.secrets.sh as a fallback) through the shared helper
# in ../lib/provider-env.sh; never source the whole file into the sandboxed
# server's env. The personal profile provides OPENROUTER_API_KEY; the work
# profile provides LITELLM_BASE_URL, LITELLM_API_KEY and HUMANIZER_MODEL.
# humanizer selects the backend from whichever is set, so generic var names
# only: the proxy host and token stay in that gitignored file. A test in
# tools/humanizer keeps judge_vars in step with kit/llm's llm.ProviderEnv.
judge_vars="OPENROUTER_API_KEY LITELLM_BASE_URL LITELLM_API_KEY HUMANIZER_MODEL"
# shellcheck source=../lib/provider-env.sh
. "$(dirname "$(realpath "$0")")/../lib/provider-env.sh"
# One variable name per word: the split is the point.
# shellcheck disable=SC2086
export_provider_env $judge_vars

# The env allow-list: HOME, PATH, TMPDIR (srt-run hands it to the child), and
# the four backend vars when set.
set -- HOME="$HOME" PATH="${PATH:-/usr/bin:/bin}"
[ -n "${TMPDIR:-}" ] && set -- "$@" TMPDIR="$TMPDIR"
for _v in $judge_vars; do
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
unset judge_vars _v _cur _host 2>/dev/null || true

cd /
exec /usr/bin/env -i "$@" \
  "$HOME/code/bin/srt-run" "$HOME/.config/srt/humanizer.json" \
  "$HOME/code/bin/humanizer" mcp
