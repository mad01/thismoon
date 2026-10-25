#!/bin/sh
# Spawn a thismoon-shipped MCP server inside its sandbox (anthropics/
# sandbox-runtime, through thismoon's srt-run launcher).
#
# Exists because a declarative MCP entry can't express this directly: the MCP
# client spawns from the session cwd, which may sit inside a read-denied
# subtree, where getcwd() is an EPERM, so cd / first.
#
# The sandbox can't filter env, so the environment is scrubbed with an env -i
# allow-list: shell secrets never enter the confined process. This example
# passes only HOME; add more only if the server genuinely needs them.
#
# srt-run reads ~/.config/srt/mytool.json (this recipe symlinks
# mytool.srt.json there) and labels the process "mytool", the settings
# file's name, so sandbox-watch can attribute its denials. A missing
# settings file stops the launch: srt fails closed.
set -eu

cd /
exec /usr/bin/env -i \
  HOME="$HOME" \
  "$HOME/code/bin/srt-run" "$HOME/.config/srt/mytool.json" \
  "$HOME/code/bin/mytool" mcp
