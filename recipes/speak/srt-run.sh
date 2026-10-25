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
#   - two widening layers over the settings, merged with jq. The settings
#     file is rendered from the process's access manifest and is the same on
#     every machine (docs/adr/0027). What one machine adds comes from:
#       - the expansion layer ~/.config/srt/<label>.local.json, if present.
#         It may only widen: add to network.allowedDomains, allowMachLookup
#         and allowUnixSockets and to filesystem.allowRead and allowWrite,
#         and set network.allowLocalBinding or enableWeakerNetworkIsolation
#         to true. Any other key, an entry that undoes a deny (an allow equal
#         to a deny entry, or ~ itself), a write reaching ~/.config/srt or
#         the ralph sources cache, or a `*` / `*:port` host refuses the
#         launch (exit 1).
#       - SRT_RUN_ALLOW_DOMAINS (comma-separated host[:port]), appended to
#         network.allowedDomains. Machine-specific hosts (a shared instance,
#         an LLM proxy) come from the wrapper's own inputs at launch. A bare
#         `*` entry is refused.
#     The merged copy goes to ~/.config/srt/merged/<label>.json (mode 600,
#     rewritten every launch) and srt reads that instead. With either layer
#     present, a missing jq or a failed merge stops the launch; there is no
#     fallback to the unmerged file. `srtspec explain <label>` prints the
#     merged policy with the layer each entry came from.
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
#   - the runtime srt itself runs on. srt is a node CLI whose process stays
#     up beside the command for its whole life: it hosts the HTTP and SOCKS
#     proxies that enforce allowedDomains and forwards signals to the
#     command. bun runs the same JS in less memory: about 48 MB resident
#     against node's 83 MB (bun 1.4.4, node 24, srt 0.0.81 on macOS).
#     SRT_RUN_RUNTIME picks bun or node; unset, bun when one is installed
#     (mise's installs dir first, then PATH) and node otherwise. Under bun
#     this script runs srt's dist/cli.js itself, since the npm bin wrapper
#     execs node. It asks mise which srt the pin selects and follows that
#     wrapper to the package's cli.js; when that fails the launch stops
#     (exit 127).
#     bun gets --no-env-file and --config=/dev/null: by default it loads
#     .env and bunfig.toml from the cwd, and a bunfig preload would run a
#     project's code in the unsandboxed parent. --no-install keeps it from
#     fetching a missing package. Either runtime prints one
#     `srt-run: <label> runtime <bun|node>` line to stderr at launch.
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

# A bun binary: the last of mise's installs (its `latest` link when mise
# keeps one), else the first on PATH. Prints nothing when there is none.
find_bun() {
  found=
  for candidate in "$HOME"/.local/share/mise/installs/bun/*/bin/bun; do
    if [ -x "$candidate" ]; then found=$candidate; fi
  done
  if [ -z "$found" ]; then found=$(command -v bun 2>/dev/null || true); fi
  printf '%s' "$found"
}

# srt's dist/cli.js. The srt on PATH is normally a mise shim, a link to the
# mise binary, so mise names the bin its pin selects, in this cwd as the shim
# would. That bin is npm's: a link to dist/cli.js, or a wrapper script in
# node_modules/.bin beside the package. Prints nothing when it isn't there.
find_srt_cli() {
  bin=$(command -v srt)
  if [ -L "$bin" ]; then
    link=$(readlink "$bin")
    case $link in /*) ;; *) link=$(dirname "$bin")/$link ;; esac
    if [ "${link##*/}" = mise ]; then
      bin=$("$link" which srt 2>/dev/null) || return 0
    fi
  fi
  hops=0
  while [ -L "$bin" ] && [ "$hops" -lt 40 ]; do
    hops=$((hops + 1))
    link=$(readlink "$bin")
    case $link in /*) bin=$link ;; *) bin=$(dirname "$bin")/$link ;; esac
  done
  case $bin in
    */cli.js) cli=$bin ;;
    *) cli=$(dirname "$bin")/../@anthropic-ai/sandbox-runtime/dist/cli.js ;;
  esac
  if [ -f "$cli" ]; then printf '%s' "$cli"; fi
}

runtime=${SRT_RUN_RUNTIME:-}
case $runtime in
  '' | bun) bun=$(find_bun) ;;
  node) bun= ;;
  *)
    echo "srt-run: SRT_RUN_RUNTIME must be bun or node, not '$runtime'" >&2
    exit 2
    ;;
esac
if [ -z "$runtime" ]; then
  if [ -n "$bun" ]; then runtime=bun; else runtime=node; fi
fi
if [ "$runtime" = bun ]; then
  if [ -z "$bun" ]; then
    echo "srt-run: SRT_RUN_RUNTIME=bun but no bun under $HOME/.local/share/mise/installs/bun or on PATH" >&2
    exit 127
  fi
  srt_cli=$(find_srt_cli)
  if [ -z "$srt_cli" ]; then
    echo "srt-run: cannot find srt's dist/cli.js behind $(command -v srt) (tried mise which srt); refusing to launch $label under bun (SRT_RUN_RUNTIME=node runs the srt wrapper)" >&2
    exit 127
  fi
fi

# The expansion layer's refusal rules, as a jq program over the settings
# file with the layer in $l. It prints one reason per line and nothing when
# the layer only widens. srtspec's layers package implements the same rules;
# recipes/speak/testdata/srt-local holds the cases both test suites run.
# shellcheck disable=SC2016
local_check='
def norm: (if . == $home then "~" elif startswith($home + "/") then "~" + .[($home | length):] else . end)
  | sub("/+$"; "");
def lit: split("/") | (map(test("[*?\\[{]")) | index(true)) as $i
  | (if $i == null then . else .[:$i] end) | join("/");
def under($d): . == $d or startswith($d + "/");
def lists: {network: ["allowedDomains", "allowMachLookup", "allowUnixSockets"],
  filesystem: ["allowRead", "allowWrite"]};
([.filesystem.denyRead[]?, .filesystem.denyWrite[]?] | map(norm)) as $denies
| $l
| if type != "object" then "the layer must be a JSON object" else
    (keys[] | select(IN("network", "filesystem", "enableWeakerNetworkIsolation", "_comment") | not)
      | "key \(.) is not allowed; the layer may only widen"),
    ("network", "filesystem") as $s | select(has($s)) | .[$s]
      | if type != "object" then "\($s) must be an object" else
          (keys[] | select((IN(lists[$s][]) or ($s == "network" and . == "allowLocalBinding")) | not)
            | "\($s).\(.) is not allowed; the layer may only widen"),
          (lists[$s][] as $k | select(has($k)) | .[$k] as $v
            | if ($v | type) != "array" or ($v | map(type == "string" and length > 0) | all | not)
              then "\($s).\($k) must be a list of non-empty strings"
              else $v[] as $item | ($item | norm) as $p
                | (if $k == "allowedDomains" and ($item == "*" or ($item | startswith("*:")))
                    then "\($s).\($k) may not hold \"\($item)\", which allows every host" else empty end),
                  (if ($k == "allowRead" or $k == "allowWrite") and ($p == "" or $p == "~")
                    then "\($s).\($k) \"\($item)\" would lift the ~ deny" else empty end),
                  (if ($k == "allowRead" or $k == "allowWrite") and ($p | IN($denies[]))
                    then "\($s).\($k) \"\($item)\" would remove the deny on \($p)" else empty end),
                  (if $k == "allowWrite" then ($p | lit) as $w
                    | ("~/.config/srt", "~/.config/ralph/sources") as $dir
                    | select(($dir | under($w)) or ($w | under($dir)))
                    | "\($s).\($k) \"\($item)\" reaches \($dir), where the policy lives"
                    else empty end)
              end),
          (if $s == "network" and has("allowLocalBinding") and .allowLocalBinding != true
            then "network.allowLocalBinding may only be set to true" else empty end)
        end,
    (if has("enableWeakerNetworkIsolation") and .enableWeakerNetworkIsolation != true
      then "enableWeakerNetworkIsolation may only be set to true" else empty end)
  end
'

# The merge, over the settings file: the checked layer in $l, then the hosts
# in $d. Lists union (sorted, as srt reads them as sets); flags only turn on.
# shellcheck disable=SC2016
local_merge='
def addlist($s; $k): if ($l[$s] | type) == "object" and ($l[$s][$k] | type) == "array"
  then .[$s][$k] = ((.[$s][$k] // []) + $l[$s][$k] | unique) else . end;
addlist("network"; "allowedDomains")
| addlist("network"; "allowMachLookup")
| addlist("network"; "allowUnixSockets")
| addlist("filesystem"; "allowRead")
| addlist("filesystem"; "allowWrite")
| if ($l.network | type) == "object" and $l.network.allowLocalBinding == true
  then .network.allowLocalBinding = true else . end
| if $l.enableWeakerNetworkIsolation == true then .enableWeakerNetworkIsolation = true else . end
| .network.allowedDomains += ($d | split(",") | map(select(length > 0)))
| .network.allowedDomains |= unique
'

local_layer="$HOME/.config/srt/$label.local.json"
if [ -e "$local_layer" ] || [ -n "${SRT_RUN_ALLOW_DOMAINS:-}" ]; then
  if [ -n "${SRT_RUN_ALLOW_DOMAINS:-}" ]; then
    case ",$SRT_RUN_ALLOW_DOMAINS," in
      *',*,'* | *',*:'*)
        echo "srt-run: SRT_RUN_ALLOW_DOMAINS may not hold a bare * entry: $SRT_RUN_ALLOW_DOMAINS" >&2
        exit 1
        ;;
    esac
  fi
  jq=$(command -v jq 2>/dev/null || true)
  if [ -z "$jq" ] && [ -x /opt/homebrew/bin/jq ]; then
    jq=/opt/homebrew/bin/jq
  fi
  if [ -z "$jq" ]; then
    echo "srt-run: $label has a layer to merge (SRT_RUN_ALLOW_DOMAINS or $local_layer) but jq is not on PATH or in /opt/homebrew/bin; refusing to launch $label with the unmerged settings" >&2
    exit 1
  fi
  layer='{}'
  if [ -e "$local_layer" ]; then
    if ! layer=$("$jq" -c . "$local_layer" 2> /dev/null) || [ -z "$layer" ]; then
      echo "srt-run: $local_layer is not valid JSON; refusing to launch $label" >&2
      exit 1
    fi
    if ! refusals=$("$jq" -r --arg home "$HOME" --argjson l "$layer" "$local_check" "$settings"); then
      echo "srt-run: cannot check $local_layer against $settings; refusing to launch $label" >&2
      exit 1
    fi
    if [ -n "$refusals" ]; then
      echo "srt-run: $local_layer may only widen the policy; refusing to launch $label:" >&2
      printf '%s\n' "$refusals" | sed 's/^/  - /' >&2
      exit 1
    fi
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
  if ! "$jq" --argjson l "$layer" --arg d "${SRT_RUN_ALLOW_DOMAINS:-}" "$local_merge" \
    "$settings" > "$tmp" ||
    ! chmod 600 "$tmp" ||
    ! mv -f "$tmp" "$merged"; then
    rm -f "$tmp"
    echo "srt-run: merging the layers into $settings failed; refusing to launch $label" >&2
    exit 1
  fi
  if [ -n "${SRT_DEBUG:-}" ]; then
    printf 'srt-run: %s allowedDomains: %s\n' "$label" \
      "$("$jq" -c '.network.allowedDomains' "$merged")" >&2
    if [ -e "$local_layer" ]; then
      printf 'srt-run: %s merged %s\n' "$label" "$local_layer" >&2
    fi
  fi
  settings=$merged
fi
unset SRT_RUN_ALLOW_DOMAINS

CLAUDE_CODE_TMPDIR=${TMPDIR:-/private/tmp}
export CLAUDE_CODE_TMPDIR
mkdir -p "$CLAUDE_CODE_TMPDIR"

printf 'srt-run: %s runtime %s\n' "$label" "$runtime" >&2
if [ "$runtime" = bun ]; then
  exec "$bun" --no-env-file --config=/dev/null --no-install "$srt_cli" \
    --settings "$settings" -- env SRT_RUN="$label" "$@"
fi
exec srt --settings "$settings" -- env SRT_RUN="$label" "$@"
