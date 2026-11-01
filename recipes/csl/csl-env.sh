#!/bin/sh
# csl-env.sh: run csl with the provider variables the csl_ask worker reads.
#
#   csl-env.sh mcp                     (an MCP client's csl server)
#
# csl_ask hands its corpus to a hosted model through kit/llm, which picks the
# provider from the environment. An MCP host carries no login shell's env, and
# baking a key into servers.json would leave a secret in a readable file. So
# pull exactly the variables kit/llm reads (llm.ProviderEnv) from the
# ralph-managed secrets file (legacy ~/.secrets.sh as a fallback) in a
# subshell, export them, and exec csl. Nothing else in the secrets file reaches
# csl, and a variable already in the environment wins. The claude CLI backend
# reads none of them: it needs only claude on PATH. A test in services/csl
# keeps the list below in step with llm.ProviderEnv.
set -eu

for _v in LITELLM_BASE_URL LITELLM_API_KEY OPENROUTER_API_KEY; do
  eval "_cur=\${$_v:-}"
  [ -n "$_cur" ] && continue
  for _sf in "$HOME/.config/ralph/secrets.sh" "$HOME/.secrets.sh"; do
    [ -f "$_sf" ] || continue
    # secrets file path is runtime-selected and read only in this subshell.
    # shellcheck disable=SC1090
    _val="$(set +eu; . "$_sf" > /dev/null 2>&1; eval "printf '%s' \"\${$_v:-}\"")"
    if [ -n "$_val" ]; then
      eval "export $_v=\"\$_val\""
      break
    fi
  done
done
unset _v _cur _sf _val 2> /dev/null || true

exec "${CSL_BIN:-$HOME/code/bin/csl}" "$@"
