#!/bin/sh
# speak-env.sh: run speak with the active provider's credentials, unsandboxed.
#
#   speak-env.sh mcp                   (an MCP client's speak server)
#
# speak-provider-env.sh exports exactly the variables the active provider
# reads, from the secrets file; this execs speak with the arguments given.
# The speak-web t-man agent starts through speak-serve.sh instead, which runs
# `speak serve` inside its sandbox.
set -eu

# shellcheck source=speak-provider-env.sh
. "$(dirname "$0")/speak-provider-env.sh"

exec "$SPEAK_BIN" "$@"
