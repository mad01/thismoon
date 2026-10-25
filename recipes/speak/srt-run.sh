#!/bin/sh
# srt-run: launch a fleet process inside anthropics/sandbox-runtime (srt).
#
#   srt-run <settings-file> <argv...>
#
# The speak recipe symlinks this script to ~/code/bin/srt-run. Every sandboxed
# MCP server and t-man service execs through it, so the fleet has one launch
# path and one marker the sandbox-watch agent can attribute denials by.
#
# The settings file is the per-process srt JSON, by convention
# ~/.config/srt/<label>.json. A settings file that is the same on every
# machine ships beside its service's recipe here; one that differs per
# machine ships from the consuming repo as an overlay (docs/adr/0006). It
# must sit outside every sandbox's allowWrite, or a confined process could
# rewrite its own policy for the next launch; no fleet sandbox may write
# ~/.config/srt. srt fails closed: a missing or invalid settings file exits
# 1 and the command never runs.
#
# What this adds over a bare `srt --settings`:
#   - a leading ~ in the settings path is expanded and the path made absolute
#     (srt expands neither)
#   - the mise shims dir is prepended to PATH, so srt resolves from a
#     launchd agent or an `env -i` wrapper, neither of which carries the
#     login shell's PATH. The system dirs come next, ahead of the caller's
#     PATH: srt starts the command through bash, refuses to run when bash is
#     not on PATH, and should get /bin/bash rather than whatever the caller's
#     PATH (or sh's default PATH under `env -i`) finds first.
#   - SRT_RUN_ALLOW_DOMAINS (comma-separated host[:port]) is merged into the
#     settings' network.allowedDomains with jq. The static file holds the
#     policy; machine-specific hosts (a shared instance, an LLM proxy) come
#     from the wrapper's own inputs at launch. The merged copy goes to
#     ~/.config/srt/merged/<label>.json (mode 600, rewritten every launch)
#     and srt reads that instead. With the variable set, a missing jq or a
#     failed merge stops the launch; there is no fallback to the unmerged
#     file. A bare `*` entry is refused.
#   - the child's TMPDIR. srt hands the child CLAUDE_CODE_TMPDIR, else
#     /tmp/claude, which it allows writes to but never creates, so temp-file
#     writers (Go's os.CreateTemp) fail. CLAUDE_CODE_TMPDIR is set to the
#     caller's TMPDIR, else /private/tmp, and created. srt does NOT add that
#     directory to the write allow-list: settings for a process that writes
#     temp files list /private/tmp and /private/var/folders in allowWrite.
#   - the command runs as `env SRT_RUN=<label> <argv...>`. srt tags every deny
#     rule with the base64 of the command's first 100 characters, so the label
#     lands in each denial. sandbox-watch decodes it to name the process, and
#     to tell fleet processes apart from agent shells that use srt too.
#
# LABEL is the settings file's name without .json
# (~/.config/srt/present.json -> present). Override it with SRT_RUN_LABEL.
# SRT_DEBUG passes through untouched (srt -d sets the same variable); with it
# set, this script also prints the merged allow-list to stderr.
#
# No cd here: wrappers keep their own `cd /`, because a cwd inside a
# read-denied subtree breaks getcwd() for the confined process.
set -eu

if [ "$#" -lt 2 ]; then
  echo "usage: srt-run <settings-file> <argv...>" >&2
  exit 2
fi

settings=$1
shift

# Matching a literal leading ~ on purpose; the caller's shell left it unexpanded.
# shellcheck disable=SC2088
case $settings in
  "~") settings=$HOME ;;
  "~/"*) settings=$HOME/${settings#"~/"} ;;
esac
case $settings in
  /*) ;;
  *) settings=$(pwd)/$settings ;;
esac

if [ -n "${SRT_RUN_LABEL:-}" ]; then
  label=$SRT_RUN_LABEL
else
  label=${settings##*/}
  label=${label%.json}
fi
# The watcher reads the label back out of the decoded command, and srt quotes
# any argument with shell-special characters, so keep it to a bare word.
case $label in
  '' | *[!A-Za-z0-9._-]*)
    echo "srt-run: label '$label' must match [A-Za-z0-9._-]+ (set SRT_RUN_LABEL)" >&2
    exit 2
    ;;
esac

PATH="$HOME/.local/share/mise/shims:/usr/bin:/bin:/usr/sbin:/sbin${PATH:+:$PATH}"
export PATH

if ! command -v srt >/dev/null 2>&1; then
  echo "srt-run: srt not found on PATH (expected the mise shim at $HOME/.local/share/mise/shims/srt; install npm:@anthropic-ai/sandbox-runtime with mise)" >&2
  exit 127
fi

if [ -n "${SRT_RUN_ALLOW_DOMAINS:-}" ]; then
  case ",$SRT_RUN_ALLOW_DOMAINS," in
    *',*,'* | *',*:'*)
      echo "srt-run: SRT_RUN_ALLOW_DOMAINS may not hold a bare * entry: $SRT_RUN_ALLOW_DOMAINS" >&2
      exit 1
      ;;
  esac
  jq=$(command -v jq 2>/dev/null || true)
  if [ -z "$jq" ] && [ -x /opt/homebrew/bin/jq ]; then
    jq=/opt/homebrew/bin/jq
  fi
  if [ -z "$jq" ]; then
    echo "srt-run: SRT_RUN_ALLOW_DOMAINS is set but jq is not on PATH or in /opt/homebrew/bin; refusing to launch $label with the unmerged settings" >&2
    exit 1
  fi
  merged_dir="$HOME/.config/srt/merged"
  merged="$merged_dir/$label.json"
  # Write beside the target and rename, so two launches of one label never
  # read a half-written file. mktemp creates it mode 600.
  if ! mkdir -p "$merged_dir" || ! chmod 700 "$merged_dir" ||
    ! tmp=$(mktemp "$merged_dir/.$label.XXXXXX"); then
    echo "srt-run: cannot write the merged settings under $merged_dir" >&2
    exit 1
  fi
  # $d is a jq variable, not a shell one.
  # shellcheck disable=SC2016
  if ! "$jq" --arg d "$SRT_RUN_ALLOW_DOMAINS" \
    '.network.allowedDomains += ($d | split(",") | map(select(length > 0))) | .network.allowedDomains |= unique' \
    "$settings" > "$tmp" ||
    ! chmod 600 "$tmp" ||
    ! mv -f "$tmp" "$merged"; then
    rm -f "$tmp"
    echo "srt-run: merging SRT_RUN_ALLOW_DOMAINS into $settings failed; refusing to launch $label" >&2
    exit 1
  fi
  if [ -n "${SRT_DEBUG:-}" ]; then
    printf 'srt-run: %s allowedDomains: %s\n' "$label" \
      "$("$jq" -c '.network.allowedDomains' "$merged")" >&2
  fi
  settings=$merged
fi
unset SRT_RUN_ALLOW_DOMAINS

CLAUDE_CODE_TMPDIR=${TMPDIR:-/private/tmp}
export CLAUDE_CODE_TMPDIR
mkdir -p "$CLAUDE_CODE_TMPDIR"

exec srt --settings "$settings" -- env SRT_RUN="$label" "$@"
