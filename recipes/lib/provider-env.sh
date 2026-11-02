#!/bin/sh
# Sourced (not run) by the launchers that hand a process its provider
# credentials: csl/csl-env.sh, humanizer/humanizer-mcp-sandbox.sh,
# present/present-shared-env.sh and speak/speak-provider-env.sh. It defines
# one function and runs nothing.
#
#   export_provider_env NAME...
#
# Neither an MCP host nor a launchd agent carries a login shell's env, and
# baking a key into servers.json or a plist would leave a secret in a readable
# file. So the function pulls exactly the named variables from the
# ralph-managed secrets file (legacy ~/.secrets.sh as a fallback), each in a
# subshell, and exports the ones it finds. Nothing else in the secrets file
# reaches the caller, a variable already in the environment wins, and a name
# neither file sets stays unset. Called with no names, it reads nothing. A NAME
# that is not a shell variable name fails the call before any secrets file is
# read.
#
# A launcher reaches this file through its own path, so it works from the
# ralph sources cache and from a checkout alike:
#
#   . "$(dirname "$(realpath "$0")")/../lib/provider-env.sh"
#
# This directory has no recipe.toml, so ralph never loads it as a recipe; it
# ships with the rest of the source checkout.

export_provider_env() {
  for _pe_name in "$@"; do
    case $_pe_name in
      '' | [0-9]* | *[!A-Za-z0-9_]*)
        printf 'export_provider_env: not a variable name: %s\n' "$_pe_name" >&2
        return 1
        ;;
    esac
  done
  for _pe_name in "$@"; do
    eval "_pe_cur=\${$_pe_name:-}"
    [ -n "$_pe_cur" ] && continue
    for _pe_file in "$HOME/.config/ralph/secrets.sh" "$HOME/.secrets.sh"; do
      [ -f "$_pe_file" ] || continue
      # secrets file path is runtime-selected and read only in this subshell.
      # shellcheck disable=SC1090
      _pe_val="$(
        set +eu
        . "$_pe_file" > /dev/null 2>&1
        eval "printf '%s' \"\${$_pe_name:-}\""
      )"
      if [ -n "$_pe_val" ]; then
        eval "export $_pe_name=\"\$_pe_val\""
        break
      fi
    done
  done
  unset _pe_name _pe_cur _pe_file _pe_val
}
