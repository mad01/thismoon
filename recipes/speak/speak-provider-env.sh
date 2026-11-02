#!/bin/sh
# Sourced (not run) by speak-env.sh and speak-serve.sh.
#
# speak reads provider keys only from the environment, and neither a launchd
# agent nor an MCP host carries a login shell's env. Baking the key into a
# plist or servers.json would leave a secret in a readable file. So ask speak
# which variables the active provider reads (`speak config env`: names only,
# nothing for the local engine) and hand those names to the shared helper in
# ../lib/provider-env.sh, which pulls exactly them from the ralph-managed
# secrets file (legacy ~/.secrets.sh as a fallback) and exports them. Nothing
# else in the secrets file reaches speak, and a variable already in the
# environment wins. Sets SPEAK_BIN for the caller's exec.
#
# The provider is resolved from the environment and the config file, so pick
# it with SPEAK_PROVIDER or the file's provider line, not a --provider flag
# after the subcommand: the env query below would not see the flag.
SPEAK_BIN="${SPEAK_BIN:-$HOME/code/bin/speak}"

# $0 is the launcher that sourced this file, which sits beside it.
# shellcheck source=../lib/provider-env.sh
. "$(dirname "$(realpath "$0")")/../lib/provider-env.sh"
# speak prints one variable name per line: the split is the point.
# shellcheck disable=SC2046
export_provider_env $("$SPEAK_BIN" config env 2> /dev/null || true)
