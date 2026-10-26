#!/usr/bin/env bash
# Wrapper for github-mcp-server: token resolution plus srt confinement.
#
# Resolves the token for the given host from `gh auth token` at startup, so
# tokens live in the macOS keychain and rotate with `gh auth refresh` without
# editing any MCP config. Token resolution happens OUTSIDE the sandbox: the
# confined server gets the token via env and can't reach the keychain or
# ~/.config/gh itself.
#
# The server then runs under ~/code/bin/srt-run (thismoon's launcher) with
# the settings file ~/.config/srt/github-mcp.json, which this recipe symlinks
# from github-mcp.srt.json: $HOME reads denied outside ~/code/bin, writes to
# temp only, and no egress host of its own. srt doesn't filter the
# environment, so it is rebuilt with an env -i allow-list and shell secrets
# never enter the confined process. cd / first: the MCP client spawns from
# the session cwd, which may sit in a read-denied subtree.
#
# Usage: github-mcp-wrapper.sh <host>
#   host: github.com, or a GitHub Enterprise host. The host and its
#   subdomains (plus *.githubusercontent.com for github.com) reach the egress
#   allow-list through SRT_RUN_ALLOW_DOMAINS, which srt-run merges in at
#   launch. One settings file serves every machine, and no hostname lands
#   in it.
set -euo pipefail
host="${1:-github.com}"
token="$(gh auth token --hostname "$host")"
toolsets="repos,issues,pull_requests"

settings="$HOME/.config/srt/github-mcp.json"
server="$HOME/code/bin/github-mcp-server"

args=(stdio --toolsets "$toolsets")
domains="$host,*.$host"
if [[ "$host" == "github.com" ]]; then
  domains+=",*.githubusercontent.com"
else
  args+=(--gh-host "https://$host")
fi
env_allow=(
  HOME="$HOME"
  PATH="${PATH:-/usr/bin:/bin}"
  GITHUB_PERSONAL_ACCESS_TOKEN="$token"
  SRT_RUN_ALLOW_DOMAINS="$domains"
)
# srt-run hands TMPDIR to the child.
[[ -n "${TMPDIR:-}" ]] && env_allow+=(TMPDIR="$TMPDIR")

cd /
exec /usr/bin/env -i "${env_allow[@]}" \
  "$HOME/code/bin/srt-run" "$settings" "$server" "${args[@]}"
