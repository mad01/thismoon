#!/bin/sh
# Sourced (not run) by present-mcp-sandbox.sh and present-serve.sh.
#
# Sharing a page needs the shared instance's URL and the author key in the
# process that pushes it: `present mcp` for present_share and `present serve`
# for the Share button. Neither runs from a login shell (MCP hosts and launchd
# agents carry no shell env), and baking the key into a plist or servers.json
# would leave a secret in a readable file. So pull exactly these two vars
# from the ralph-managed secrets file (legacy ~/.secrets.sh as a fallback)
# through the shared helper in ../lib/provider-env.sh; never source the whole
# file into the caller. A var already in the environment wins. present enables
# sharing only when both are set and warns at startup about a lone one, so an
# unset pair is simply "no sharing".
#
# Both launchers run present under srt, whose static settings name no host.
# So the shared instance's host[:port] is exported as SRT_RUN_ALLOW_DOMAINS,
# which srt-run merges into network.allowedDomains at launch:
# https://user@share.example.com:8443/base?q gives share.example.com:8443.
# No shared URL, no egress beyond localhost.

# $0 is the launcher that sourced this file, which sits beside it.
# shellcheck source=../lib/provider-env.sh
. "$(dirname "$(realpath "$0")")/../lib/provider-env.sh"
export_provider_env PRESENT_SHARED_URL PRESENT_AUTHOR_KEY
if [ -n "${PRESENT_SHARED_URL:-}" ]; then
  _host=${PRESENT_SHARED_URL#*://}
  _host=${_host%%/*}
  _host=${_host%%\?*}
  _host=${_host%%#*}
  _host=${_host##*@}
  if [ -n "$_host" ]; then
    SRT_RUN_ALLOW_DOMAINS=$_host
    export SRT_RUN_ALLOW_DOMAINS
  fi
fi
unset _host 2> /dev/null || true
