#!/bin/sh
# csl-env.sh: run csl with the provider variables the csl_ask worker reads.
#
#   csl-env.sh mcp                     (an MCP client's csl server)
#
# csl_ask hands its corpus to a hosted model through kit/llm, which picks the
# provider from the environment. An MCP host carries no login shell's env, and
# baking a key into servers.json would leave a secret in a readable file. So
# pull exactly the variables kit/llm reads (llm.ProviderEnv) from the
# ralph-managed secrets file (legacy ~/.secrets.sh as a fallback) through the
# shared helper in ../lib/provider-env.sh, export them, and exec csl. Nothing
# else in the secrets file reaches csl, and a variable already in the
# environment wins. The claude CLI backend reads none of them: it needs only
# claude on PATH. A test in services/csl keeps the list below in step with
# llm.ProviderEnv.
set -eu

# shellcheck source=../lib/provider-env.sh
. "$(dirname "$(realpath "$0")")/../lib/provider-env.sh"
export_provider_env LITELLM_BASE_URL LITELLM_API_KEY OPENROUTER_API_KEY

exec "${CSL_BIN:-$HOME/code/bin/csl}" "$@"
