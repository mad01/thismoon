#!/usr/bin/env bash
# Wrapper for an agent's shell tool: one command, confined by srt.
#
# Usage: agent-shell.sh <command>
#   The agent's tool calls it with the command string as the one argument,
#   where it would otherwise call `bash -c <command>`, and the workspace as
#   the working directory. stdout, stderr and the exit code are the
#   command's.
#
# The command runs under ~/code/bin/srt-run (thismoon's launcher) with the
# settings file ~/.config/srt/agent-shell.json, which this recipe symlinks
# from agent-shell.srt.json: $HOME reads denied outside the code roots and
# toolchains, writes to the workspace and temp, no network host of its own.
# Relative entries in the settings ("." and the secret globs) resolve
# against the working directory, so there is no `cd /` here: the workspace
# stays the cwd.
#
# srt doesn't filter the environment, so it is rebuilt with env -i from an
# allow-list, and a secret exported in the agent's own environment never
# enters the command. SRT_RUN_ALLOW_DOMAINS is on the list: the caller sets
# it (comma-separated host[:port]) to open egress for one command, and
# srt-run merges it at launch. Unset, the command has no network.
#
# git under srt resolves to /usr/bin/git, an xcrun shim that needs a readable
# developer directory. An Xcode under $HOME is unreadable inside the sandbox,
# so the Command Line Tools stand in when the machine has them and the
# caller set no DEVELOPER_DIR (thismoon docs/SANDBOXING.md, "git inside the
# sandbox").
set -euo pipefail
if [[ $# -ne 1 ]]; then
  echo "usage: agent-shell.sh <command>" >&2
  exit 2
fi
cmd="$1"
settings="$HOME/.config/srt/agent-shell.json"

keep=(
  HOME PATH USER LOGNAME SHELL TERM LANG LC_ALL LC_CTYPE TMPDIR
  NO_COLOR CLICOLOR COLORTERM PAGER
  DEVELOPER_DIR SDKROOT GOPATH GOCACHE GOMODCACHE GOFLAGS
  SRT_RUN_ALLOW_DOMAINS SRT_RUN_RUNTIME SRT_DEBUG
)
env_allow=()
for name in "${keep[@]}"; do
  if [[ -n "${!name:-}" ]]; then
    env_allow+=("$name=${!name}")
  fi
done
clt=/Library/Developer/CommandLineTools
if [[ -z "${DEVELOPER_DIR:-}" && -d "$clt" ]]; then
  env_allow+=("DEVELOPER_DIR=$clt")
fi

exec /usr/bin/env -i "${env_allow[@]}" \
  "$HOME/code/bin/srt-run" "$settings" /bin/bash -c "$cmd"
