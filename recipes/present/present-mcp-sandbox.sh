#!/bin/sh
# Spawn the present MCP server inside its seatbelt profile.
#
# Exists because servers.json can't express this directly:
#   - sync_mcp.py only expands ~ in `command`, not args, so the profile
#     path can't be passed portably as an argument
#   - the MCP client spawns from the session cwd, which may sit inside a
#     read-denied subtree — getcwd() there is an EPERM, so cd / first
#
# Seatbelt can't filter env, so the environment is scrubbed with an env -i
# allow-list — shell secrets never enter the confined process. present needs
# HOME plus its own PRESENT_* config (supplied by the `env` block in
# servers.json; defaulted here so the wrapper also works standalone).
set -eu

cd /
exec /usr/bin/env -i \
  HOME="$HOME" \
  PRESENT_WORKDIR="${PRESENT_WORKDIR:-$HOME/.config/present}" \
  PRESENT_PORT="${PRESENT_PORT:-7423}" \
  PRESENT_BASE_URL="${PRESENT_BASE_URL:-http://present.this}" \
  /usr/bin/sandbox-exec \
  -D "HOME=$HOME" \
  -f "$HOME/.config/ralph/sources/thismoon/recipes/present/present.sb" \
  "$HOME/code/bin/present" mcp
