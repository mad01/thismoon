#!/bin/sh
# Sourced (not run) by present-mcp-sandbox.sh and present-serve.sh.
#
# Sharing a page needs the shared instance's URL and the author key in the
# process that pushes it: `present mcp` for present_share and `present serve`
# for the Share button. Neither runs from a login shell (MCP hosts and launchd
# agents carry no shell env), and baking the key into a plist or servers.json
# would leave a secret in a readable file. So pull exactly these two vars
# from the ralph-managed secrets file (legacy ~/.secrets.sh as a fallback) in
# a subshell; never source the whole file into the caller. A var already in
# the environment wins. present enables sharing only when both are set and
# warns at startup about a lone one, so an unset pair is simply "no sharing".
for _sf in "$HOME/.config/ralph/secrets.sh" "$HOME/.secrets.sh"; do
  [ -f "$_sf" ] || continue
  for _v in PRESENT_SHARED_URL PRESENT_AUTHOR_KEY; do
    eval "_cur=\${$_v:-}"
    [ -n "$_cur" ] && continue
    # secrets file path is runtime-selected and read only in this subshell.
    # shellcheck disable=SC1090
    _val="$(set +eu; . "$_sf" >/dev/null 2>&1; eval "printf '%s' \"\${$_v:-}\"")"
    [ -n "$_val" ] && eval "export $_v=\"\$_val\""
  done
done
unset _sf _v _cur _val 2>/dev/null || true
