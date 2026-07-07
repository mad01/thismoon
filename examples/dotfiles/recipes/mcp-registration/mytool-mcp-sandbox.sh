#!/bin/sh
# Spawn a thismoon-shipped MCP server inside its seatbelt profile.
#
# Exists because a declarative MCP entry can't express this directly:
#   - the profile path can't be passed portably as an argument
#   - the MCP client spawns from the session cwd, which may sit inside a
#     read-denied subtree — getcwd() there is an EPERM, so cd / first
#
# Seatbelt can't filter env, so the environment is scrubbed with an env -i
# allow-list — shell secrets never enter the confined process. This example
# passes only HOME; add more only if the server genuinely needs them.
set -eu

cd /
exec /usr/bin/env -i \
  HOME="$HOME" \
  /usr/bin/sandbox-exec \
  -D "HOME=$HOME" \
  -f "$HOME/code/src/github.com/you/dotfiles/recipes/mcp-registration/mytool.sb" \
  "$HOME/code/bin/mytool" mcp
