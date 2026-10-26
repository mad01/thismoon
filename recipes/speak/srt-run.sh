#!/bin/sh
# srt-run: the fleet's sandbox launcher, by its original path.
#
#   srt-run <settings-file> <argv...>
#
# The work is `srtspec run` (Go, tools/srtspec): it merges the machine's
# expansion layer and SRT_RUN_ALLOW_DOMAINS into the settings, picks bun or
# node, and execs srt with the command tagged `env SRT_RUN=<label> <argv>`.
# Every sandboxed MCP server and t-man service still execs ~/code/bin/srt-run,
# which the speak recipe links to this file, so the launch path, the marker
# sandbox-watch attributes denials by, and the exit codes (2 usage, 127
# missing runtime, 1 refused layer) are unchanged. The srtspec recipe installs
# the binary to ~/code/bin; launchd carries no login PATH, so the path is
# spelled out. No cd here: wrappers keep their own `cd /`.
set -eu

srtspec="$HOME/code/bin/srtspec"
if [ ! -x "$srtspec" ]; then
  echo "srt-run: $srtspec not found; ralph up installs it from the thismoon srtspec recipe" >&2
  exit 127
fi
exec "$srtspec" run "$@"
