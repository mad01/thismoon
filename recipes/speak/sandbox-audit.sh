#!/bin/bash
# Audit seatbelt (sandbox-exec) denials from the unified log.
#
#   sandbox-audit.sh stream         # live denials, all sandboxed processes
#   sandbox-audit.sh show [1h]      # retroactive denials (default last 1h)
#
# Three uses: fixing breakage (a denial broke synthesis), ratcheting the
# profile tighter (flip an allow to deny, watch for fallout), and compromise
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

case "${1:-stream}" in
  stream)
    exec /usr/bin/log stream --style compact \
      --predicate 'eventMessage CONTAINS "deny(1)"'
    ;;
  show)
    exec /usr/bin/log show --last "${2:-1h}" --style compact \
      --predicate 'eventMessage CONTAINS "deny(1)"'
    ;;
  *)
    echo "usage: $(basename "$0") stream | show [1h|24h|7d]" >&2
    exit 2
    ;;
esac
