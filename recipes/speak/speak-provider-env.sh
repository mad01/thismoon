#!/bin/sh
# Sourced (not run) by speak-env.sh and speak-serve.sh.
#
# speak reads provider keys only from the environment, and neither a launchd
# agent nor an MCP host carries a login shell's env. Baking the key into a
# plist or servers.json would leave a secret in a readable file. So ask speak
# which variables the active provider reads (`speak config env`: names only,
# nothing for the local engine), pull exactly those from the ralph-managed
# secrets file (legacy ~/.secrets.sh as a fallback) in a subshell, and export
# them. Nothing else in the secrets file reaches speak, and a variable already
# in the environment wins. Sets SPEAK_BIN for the caller's exec.
#
# The provider is resolved from the environment and the config file, so pick
# it with SPEAK_PROVIDER or the file's provider line, not a --provider flag
# after the subcommand: the env query below would not see the flag.
SPEAK_BIN="${SPEAK_BIN:-$HOME/code/bin/speak}"

for _v in $("$SPEAK_BIN" config env 2> /dev/null || true); do
  eval "_cur=\${$_v:-}"
  [ -n "$_cur" ] && continue
  for _sf in "$HOME/.config/ralph/secrets.sh" "$HOME/.secrets.sh"; do
    [ -f "$_sf" ] || continue
    # secrets file path is runtime-selected and read only in this subshell.
    # shellcheck disable=SC1090
    _val="$(set +eu; . "$_sf" > /dev/null 2>&1; eval "printf '%s' \"\${$_v:-}\"")"
    if [ -n "$_val" ]; then
      eval "export $_v=\"\$_val\""
      break
    fi
  done
done
unset _v _cur _sf _val 2> /dev/null || true
