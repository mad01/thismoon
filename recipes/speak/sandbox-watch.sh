#!/bin/bash
# sandbox-watch — best-effort seatbelt-denial notifier with triage context
# and a silence-list for expected/benign denials.
#
# Tails the LIVE unified-log stream for Sandbox denials by the watched process
# names, parses each into "process(pid) · operation · target", batches per
# window (a denied call in a retry loop emits thousands of lines/sec — raw
# line->banner would melt Notification Center), and notifies once per window.
# Every denial is appended to LOGFILE tagged ALERT or IGNORED; only ALERTs
# raise a banner. Each banner is also mirrored to NOTIFYLOG (one NOTIFIED
# header + the batch's unique denials) — banners can't be copied from
# Notification Center, so this is the record to read back when one fires.
#
# BEST-EFFORT, NOT A COMPLETE AUDIT TRAIL — read this before trusting it.
# Denials DO stream live (default level) via `log stream` with the
# `eventMessage CONTAINS "deny(1)"` predicate. But the kernel rate-limits and
# dedupes repeated identical denials ("N duplicate report for ..."), and
# `log show` after the fact is unreliable for them, so this is a live tap, not
# a complete ledger. Enforcement is solid (the access IS blocked); observ-
# ability is lossy on repeats. This catches PERSISTENT denials from long-lived
# sandboxed services (the real threat: a compromised package in the running
# engine repeatedly probing/exfiltrating) far better than one-shot probes. For
# a guaranteed audit trail you need Apple's Endpoint Security framework (signed
# system extension) — out of scope here.
#
# Why a LIVE stream: the original watcher used `sender == "Sandbox"` /
# `BEGINSWITH "Sandbox: "`, which matches NOTHING — it silently caught no
# denials. The working predicate is `eventMessage CONTAINS "deny(1)"` at
# DEFAULT level (adding --level debug actually drops these events). The second
# bug was exiting the whole agent when the stream died (KeepAlive restart left
# a blind gap); here an inner loop restarts the stream in place.
#
# Silence-list: SANDBOX_WATCH_IGNORE_FILE (default sandbox-ignore.conf beside
# this script) holds ERE patterns; a denial matching any is logged IGNORED and
# does not notify. A second, machine-local list (SANDBOX_WATCH_LOCAL_IGNORE_FILE,
# default ~/.config/sandbox-watch/ignore.conf) is checked too — machine-specific
# silences go there, NOT beside the script: this script ships in a ralph sources
# cache, where local edits dirty the clone and die on the next sync. Both files
# are re-read every batch, so edits apply without a restart.
# See "Triaging a denial" in recipes/speak/CLAUDE.md.
#
# Generic: one watcher covers every sandboxed t-man service — add a new
# service's process name to SANDBOX_WATCH_PROCESSES (comma-separated).
#
# Gotcha: notifications from a launchd agent are attributed to "Script
# Editor" — allow once in System Settings -> Notifications -> Script Editor.
set -u

PROCESSES="${SANDBOX_WATCH_PROCESSES:-python,Python}"
LOGFILE="${SANDBOX_WATCH_LOG:-$HOME/.local/share/speak/logs/sandbox-denials.log}"
NOTIFYLOG="${SANDBOX_WATCH_NOTIFY_LOG:-$HOME/.local/share/speak/logs/sandbox-notifications.log}"
WINDOW="${SANDBOX_WATCH_WINDOW:-60}"   # notification batch cadence (seconds)
TICK="${SANDBOX_WATCH_TICK:-5}"        # stream read timeout / flush check (seconds)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IGNORE_FILE="${SANDBOX_WATCH_IGNORE_FILE:-$SCRIPT_DIR/sandbox-ignore.conf}"
LOCAL_IGNORE_FILE="${SANDBOX_WATCH_LOCAL_IGNORE_FILE:-$HOME/.config/sandbox-watch/ignore.conf}"

mkdir -p "$(dirname "$LOGFILE")"

IFS=',' read -r -a procs <<< "$PROCESSES"

# Match on the PROCESS NAME ONLY, as a prefix (so a "python" entry still
# catches python3.14). This used to substring-match the whole message, which
# let unrelated system processes through whenever a watched name happened to
# appear in the denial target: studentd's "property:ean-storage-present" and
# softwareupdated's "panicmedic-auxkc-present" both matched "present", and
# GamePolicyAgent reading ".../Python3.framework/..." matched "Python".
# Message shape is "Sandbox: <proc>(<pid>) deny(<n>) <op> <target>".
matches_watched() {
  local msg=$1 proc p
  proc=${msg#Sandbox: }   # "python3.14(123) deny(1) file-read-data /path"
  proc=${proc%% *}        # "python3.14(123)"
  proc=${proc%%(*}        # "python3.14"
  [ -n "$proc" ] || return 1
  for p in "${procs[@]}"; do
    case "$proc" in "$p"*) return 0 ;; esac
  done
  return 1
}

# match (silence) if $1 matches any active ERE in either ignore file
is_ignored() {
  local msg=$1 pat f
  for f in "$IGNORE_FILE" "$LOCAL_IGNORE_FILE"; do
    [ -r "$f" ] || continue
    while IFS= read -r pat; do
      case "$pat" in ''|'#'*) continue ;; esac
      printf '%s' "$msg" | /usr/bin/grep -Eq -- "$pat" && return 0
    done < "$f"
  done
  return 1
}

# "Sandbox: python3.14(123) deny(1) file-read-data /path" (slash-unescaped)
# -> "python3.14(123) · file-read-data · /path"
summarize() {
  printf '%s' "$1" | /usr/bin/sed -E \
    -e 's#^Sandbox: ([^ ]+) deny\([0-9]+\) ([a-z*-]+) ?(.*)$#\1 · \2 · \3#'
}

# escape a value for embedding inside a JSON string literal (backslash, quote,
# tab; newlines become \n so a multi-line batch fits in one field)
json_escape() {
  local s=$1
  s=${s//\\/\\\\}
  s=${s//\"/\\\"}
  s=${s//$'\t'/\\t}
  s=${s//$'\n'/\\n}
  printf '%s' "$s"
}

batch_count=0
batch_first=""
batch_unique=""        # newline-separated unique ALERT summaries this window
batch_unique_n=0
UNIQUE_CAP=10          # cap per notification record; the full stream is in LOGFILE
batch_started=0        # epoch when the first ALERT entered the current batch

flush() {
  local now ts
  now=$(/bin/date +%s)
  if [ "$batch_count" -gt 0 ] && [ $((now - batch_started)) -ge "$WINDOW" ]; then
    /usr/bin/osascript -e "display notification \"${batch_first:0:160}\" with title \"sandbox: ${batch_count} new denial(s)\" subtitle \"tail ${LOGFILE/#$HOME/~}\" sound name \"Funk\"" >/dev/null 2>&1 || true
    # Also archive the banner as an event (events.this). Best-effort and
    # fire-and-forget — never block or fail the watcher if events is down.
    # POST via /usr/bin/curl, NOT the `events` CLI: launchd agents get the bare
    # default PATH, which lacks the local bin dir, so a `command -v events`
    # guard silently skips the emit on every flush (the original CLI version
    # archived nothing, ever). Pull the process and operation out of the first
    # summary ("proc(pid) · operation · target") so the event is filterable by
    # process and denial type; message carries the batch's unique denials, not
    # just the first. These are ALERT batches (IGNORED denials never notify).
    local ev_proc ev_rest ev_op
    ev_proc="${batch_first%% · *}"   # "python3.14(123)"
    ev_proc="${ev_proc%%(*}"          # "python3.14"
    ev_rest="${batch_first#* · }"     # "file-read-data · /path"
    ev_op="${ev_rest%% · *}"          # "file-read-data"
    /usr/bin/curl -m 2 -s -o /dev/null -X POST \
      -H 'Content-Type: application/json' \
      --data "{\"source\":\"sandbox-watch\",\"level\":\"warn\",\"title\":\"${batch_count} sandbox denial(s)\",\"message\":\"$(json_escape "${batch_unique:0:2000}")\",\"tags\":{\"unique\":\"${batch_unique_n}\",\"process\":\"$(json_escape "$ev_proc")\",\"op\":\"$(json_escape "$ev_op")\",\"status\":\"alert\"}}" \
      "${EVENTS_BASE_URL:-http://127.0.0.1:7430}/api/events" >/dev/null 2>&1 &
    # Mirror what the banner covered to NOTIFYLOG — banners can't be copied,
    # this record can. One header line + the batch's unique denials, indented.
    ts=$(/bin/date '+%Y-%m-%d %H:%M:%S')
    {
      printf '%s\tNOTIFIED\t%d denial(s), %d unique\n' "$ts" "$batch_count" "$batch_unique_n"
      printf '%s\n' "$batch_unique" | /usr/bin/sed 's/^/\t/'
      [ "$batch_unique_n" -ge "$UNIQUE_CAP" ] && printf '\t… unique cap (%d) hit; full stream in %s\n' "$UNIQUE_CAP" "$LOGFILE"
    } >> "$NOTIFYLOG"
    batch_count=0
    batch_first=""
    batch_unique=""
    batch_unique_n=0
    batch_started=0
  fi
}

handle_line() {
  local line=$1 msg summary ts
  case "$line" in *'deny('*) ;; *) return ;; esac
  msg=$(printf '%s' "$line" | /usr/bin/sed -n 's/.*"eventMessage":"\(Sandbox: [^"]*\)".*/\1/p' | /usr/bin/sed 's#\\/#/#g')
  [ -z "$msg" ] && return
  matches_watched "$msg" || return
  summary=$(summarize "$msg")
  ts=$(/bin/date '+%Y-%m-%d %H:%M:%S')
  if is_ignored "$msg"; then
    printf '%s\tIGNORED\t%s\n' "$ts" "$summary" >> "$LOGFILE"
  else
    printf '%s\tALERT\t%s\n' "$ts" "$summary" >> "$LOGFILE"
    [ "$batch_count" -eq 0 ] && batch_started=$(/bin/date +%s)
    batch_count=$((batch_count + 1))
    [ -z "$batch_first" ] && batch_first=$summary
    if [ "$batch_unique_n" -lt "$UNIQUE_CAP" ] && \
       ! printf '%s\n' "$batch_unique" | /usr/bin/grep -Fxq -- "$summary"; then
      batch_unique="${batch_unique:+$batch_unique
}$summary"
      batch_unique_n=$((batch_unique_n + 1))
    fi
  fi
}

# Outer loop: (re)start the live stream forever. Inner loop: read with a tick
# timeout so batched notifications flush even when the stream is quiet; on EOF
# (stream died) break to restart in place — the agent never exits, so there's
# no KeepAlive-restart gap.
main() {
  while :; do
    while :; do
      if IFS= read -r -t "$TICK" line <&3; then
        handle_line "$line"
      else
        [ $? -le 128 ] && break   # EOF: stream ended; >128 = tick timeout
      fi
      flush
    done 3< <(/usr/bin/log stream --style ndjson \
      --predicate 'eventMessage CONTAINS "deny(1)"')
    flush
    sleep 1
  done
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main
fi
