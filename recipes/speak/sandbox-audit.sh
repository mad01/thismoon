#!/bin/bash
# Audit sandbox denials from the unified log.
#
#   sandbox-audit.sh stream         # live denials, all sandboxed processes
#   sandbox-audit.sh show [1h]      # retroactive denials (default last 1h);
#                                   # best-effort: `log show` drops many of
#                                   # them, so prefer `stream` while probing
#
# Output is the log's compact style. A denial raised under srt carries a
# CMD64 tag, which the log prints on the line after it; this script replaces
# that line with the decoded srt-run label ("    srt: present"), or with
# "    srt: foreign" when the command has no SRT_RUN marker (an agent's own
# bash sandbox). A legacy seatbelt profile's marker line ("SRT_RUN=speak-tts")
# becomes "    seatbelt: speak-tts". The decoder is sandbox-watch.sh's,
# sourced from beside this script. Denials with no tag line are system
# daemons' own profiles.
#
# Three uses: fixing breakage (a denial broke synthesis), ratcheting the
# settings tighter (drop an allow, watch for fallout), and compromise
# detection — an unexpected denial burst after a pin bump (e.g. probes of
# ~/.ssh or the network) is an indicator, not a config bug.
#
# Predicate: `eventMessage CONTAINS "deny(1)"` at DEFAULT level is the only
# form that matches. `sender == "Sandbox"` / `BEGINSWITH "Sandbox: "` match
# NOTHING, and --level debug DROPS the events (same lesson as
# sandbox-watch.sh — this script shipped with the dead predicate until the
# docs pass caught it).
#
# `show` is best-effort only: the unified log often fails to persist denials,
# so retro queries can come back empty while the live stream sees them. To
# audit a workload (e.g. after a pin bump), prefer `stream` in one terminal
# while exercising the service, or read sandbox-watch's ledger at
# ~/.local/share/speak/logs/sandbox-denials.log.
set -u

# shellcheck source=recipes/speak/sandbox-watch.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/sandbox-watch.sh"

annotate() {
  local line
  while IFS= read -r line; do
    case "$line" in
      CMD64_*)
        if denial_label "$line" && [ -n "$LABEL" ]; then
          printf '    srt: %s\n' "$LABEL"
        else
          printf '    srt: foreign\n'
        fi
        ;;
      SRT_RUN=*) printf '    seatbelt: %s\n' "${line#SRT_RUN=}" ;;
      *) printf '%s\n' "$line" ;;
    esac
  done
}

case "${1:-stream}" in
  stream)
    /usr/bin/log stream --style compact \
      --predicate 'eventMessage CONTAINS "deny(1)"' | annotate
    ;;
  show)
    /usr/bin/log show --last "${2:-1h}" --style compact \
      --predicate 'eventMessage CONTAINS "deny(1)"' | annotate
    ;;
  *)
    echo "usage: $(basename "$0") stream | show [1h|24h|7d]" >&2
    exit 2
    ;;
esac
