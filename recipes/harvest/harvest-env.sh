#!/bin/sh
# harvest-env.sh: run harvest with the provider variables its model step reads.
#
#   harvest-env session-end            (the Claude Code SessionEnd hook)
#
# A SessionEnd hook carries no login shell's env, and baking a key into the
# Claude Code settings file would leave a secret in a readable file. So pull
# exactly the variables kit/llm reads (llm.ProviderEnv) from the
# ralph-managed secrets file (legacy ~/.secrets.sh as a fallback) through the
# shared helper in ../lib/provider-env.sh, export them, and exec harvest.
# Nothing else in the secrets file reaches harvest, and a variable already in
# the environment wins. harvest hands them on to its model step alone; the
# steps that read the transcript and write the vault never see them. The
# claude CLI backend reads none of them: it needs only claude on PATH. A test
# in tools/harvest keeps the list below in step with llm.ProviderEnv.
set -eu

# shellcheck source=../lib/provider-env.sh
. "$(dirname "$(realpath "$0")")/../lib/provider-env.sh"
export_provider_env LITELLM_BASE_URL LITELLM_API_KEY OPENROUTER_API_KEY

exec "${HARVEST_BIN:-$HOME/code/bin/harvest}" "$@"
