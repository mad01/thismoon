#!/bin/sh
# Launch `present serve` for the t-man agent, inside its sandbox
# (anthropics/sandbox-runtime, launched through srt-run).
#
# The Share button in the page view needs PRESENT_SHARED_URL and
# PRESENT_AUTHOR_KEY in the serve process. present-shared-env.sh reads exactly
# those two from the ralph-managed secrets file, here, outside the sandbox, so
# the sandbox never sees the secrets file and the t-man definition carries no
# secret. It also hands srt-run the shared host as SRT_RUN_ALLOW_DOMAINS, so
# the share push is the only egress beyond localhost. Every argument passes
# through to `present serve` unchanged.
#
# The settings are ~/.config/srt/present-serve.json, which the present recipe
# links from present-serve.srt.json. The label (present-serve) differs from
# the MCP server's (present), so sandbox-watch tells the two apart. serve's
# terminal sessions run inside this sandbox too, and it grants no
# pseudo-terminal, so they do not start (docs/adr/0025, 2026-10-25 amendment).
set -eu

# shellcheck source=present-shared-env.sh
. "$(dirname "$0")/present-shared-env.sh"

cd /
exec "$HOME/code/bin/srt-run" "$HOME/.config/srt/present-serve.json" \
  "$HOME/code/bin/present" serve "$@"
