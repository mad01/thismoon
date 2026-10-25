#!/bin/sh
# Spawn the present MCP server inside its sandbox (anthropics/sandbox-runtime,
# launched through srt-run).
#
# Exists because servers.json can't express the whole launch. agent-sync
# expands a leading ~ in `command` and in each arg, so the settings path alone
# would fit there. The working directory wouldn't: the MCP client spawns
# from the session cwd, which may sit inside a read-denied subtree.
# getcwd() there is an EPERM, so the wrapper does cd / first.
#
# The sandbox can't filter env, so the environment is scrubbed with an env -i
# allow-list and shell secrets never enter the confined process. present
# needs HOME plus its own PRESENT_* config (supplied by the `env` block in
# servers.json; defaulted here so the wrapper also works standalone), and
# for present_share the shared URL and author key, which
# present-shared-env.sh reads from the ralph-managed secrets file. Those two
# pass through only when set, as do TMPDIR (so the sandboxed process keeps
# the per-user temp dir; srt-run falls back to /private/tmp without it) and
# SRT_DEBUG for srt's own debug output.
#
# The sandbox settings are ~/.config/srt/present.json. This repo does not ship
# them: the consuming repo provides the file as part of its machine-private overlay
# (docs/adr/0006), next to the MCP registration that points here. It lives
# outside present's page store on purpose: the sandbox may write the store,
# and must never be able to rewrite its own policy. Without the file srt
# exits 1 and present never starts (fail closed). docs/SANDBOXING.md has a
# worked settings example; present needs allowLocalBinding for the events
# POST to 127.0.0.1:7430 and writes to its page store and temp dir.
#
# The static file keeps network.allowedDomains empty. The shared instance's
# host is machine-specific and comes from PRESENT_SHARED_URL:
# present-shared-env.sh derives host[:port] from it as SRT_RUN_ALLOW_DOMAINS,
# and this wrapper passes that through to srt-run, which merges it into the
# allow-list at launch. No shared URL, no egress beyond localhost.
set -eu

# shellcheck source=present-shared-env.sh
. "$(dirname "$0")/present-shared-env.sh"

set -- \
  HOME="$HOME" \
  PRESENT_WORKDIR="${PRESENT_WORKDIR:-$HOME/.config/present}" \
  PRESENT_PORT="${PRESENT_PORT:-7423}" \
  PRESENT_BASE_URL="${PRESENT_BASE_URL:-http://present.this}"
if [ -n "${PRESENT_SHARED_URL:-}" ]; then
  set -- "$@" PRESENT_SHARED_URL="$PRESENT_SHARED_URL"
fi
if [ -n "${PRESENT_AUTHOR_KEY:-}" ]; then
  set -- "$@" PRESENT_AUTHOR_KEY="$PRESENT_AUTHOR_KEY"
fi
if [ -n "${TMPDIR:-}" ]; then
  set -- "$@" TMPDIR="$TMPDIR"
fi
if [ -n "${SRT_DEBUG:-}" ]; then
  set -- "$@" SRT_DEBUG="$SRT_DEBUG"
fi
if [ -n "${SRT_RUN_ALLOW_DOMAINS:-}" ]; then
  set -- "$@" SRT_RUN_ALLOW_DOMAINS="$SRT_RUN_ALLOW_DOMAINS"
fi

cd /
exec /usr/bin/env -i "$@" \
  "$HOME/code/bin/srt-run" "$HOME/.config/srt/present.json" \
  "$HOME/code/bin/present" mcp
