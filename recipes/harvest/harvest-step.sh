#!/bin/sh
# harvest-step.sh: run one step of a harvest under srt.
#
#   harvest-step.sh <harvest binary> step <name> <args...>
#
# `harvest run` starts its prepare, complete and apply steps through this
# script (HARVEST_LAUNCHER), so each one runs confined by the settings in
# ~/.config/srt/harvest.json. harvest builds the step's environment itself,
# from an allow-list, and passes the model provider's host to the complete
# step alone as SRT_RUN_ALLOW_DOMAINS, which srt-run merges in at launch.
# The script adds nothing to that environment: it only names the settings.
set -eu

if [ "$#" -lt 1 ]; then
  echo "usage: harvest-step.sh <harvest binary> step <name> <args...>" >&2
  exit 2
fi

exec "$HOME/code/bin/srt-run" "$HOME/.config/srt/harvest.json" "$@"
