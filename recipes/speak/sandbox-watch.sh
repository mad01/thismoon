#!/bin/bash
# sandbox-watch: best-effort sandbox-denial notifier with triage context
# and a silence-list for expected/benign denials.
#
# Tails the LIVE unified-log stream for Sandbox denials raised by fleet
# processes, parses each into "label · process(pid) · operation · target",
# batches per window (a denied call in a retry loop emits thousands of
# lines/sec, so raw line->banner would melt Notification Center), and notifies
# once per window. Every fleet denial is appended to LOGFILE tagged ALERT or
# IGNORED; only ALERTs raise a banner. Each banner is also mirrored to
# NOTIFYLOG (one NOTIFIED header + the batch's unique denials): banners can't
# be copied from Notification Center, so this is the record to read back when
# one fires. Each banner is archived to events.this as well.
#
# Attribution. Fleet processes run under anthropics/sandbox-runtime (srt)
# through srt-run, which execs the command as `env SRT_RUN=<label> <argv...>`.
# srt tags every deny rule with "CMD64_<base64 of the command's first 100
# chars>_END_<session>_SBX", which the unified log appends to the denial after
# a newline. A legacy service that srt can't express (speak-tts needs iokit
# for Metal) keeps a hand-written seatbelt profile under t-man's
# --sandbox-profile, whose deny rules carry (with message "SRT_RUN=<label>"),
# so its denials end in a plain "\nSRT_RUN=<label>". So a denial line is:
#   - a FLEET denial when it carries CMD64_ and the decoded command holds
#     SRT_RUN=<label>, or, with no CMD64_ tag, when the message contains the
#     plain SRT_RUN=<label> marker: ALERT, or IGNORED when an ignore pattern
#     matches;
#   - FOREIGN when it carries CMD64_ without the marker: an agent's own bash
#     sandbox (Claude Code and pi both use srt). Logged IGNORED with reason
#     "foreign", never notified;
#   - dropped when it has neither (system daemons' own profiles).
# The kernel's "N duplicate reports for Sandbox: ..." summaries are dropped
# too, as they always were: they don't start with "Sandbox: ".
#
# BEST-EFFORT, NOT A COMPLETE AUDIT TRAIL — read this before trusting it.
# Denials DO stream live (default level) via `log stream` with the
# `eventMessage CONTAINS "deny(1)"` predicate. But the kernel rate-limits and
# dedupes repeated identical denials ("N duplicate report for ..."), throttles
# a binary that keeps violating (its later denials go unreported for minutes,
# new paths included), and `log show` after the fact is unreliable for them,
# so this is a live tap, not a complete ledger. Enforcement is solid (the access IS blocked); observ-
# ability is lossy on repeats. This catches PERSISTENT denials from long-lived
# sandboxed services (the real threat: a compromised package in the running
# engine repeatedly probing/exfiltrating) far better than one-shot probes. For
# a guaranteed audit trail you need Apple's Endpoint Security framework (signed
# system extension), which is out of scope here. The srt CLI records no violations
# itself (upstream issue #582), so this watcher is the only tap.
#
# Why a LIVE stream: the original watcher used `sender == "Sandbox"` /
# `BEGINSWITH "Sandbox: "`, which matches NOTHING — it silently caught no
# denials. The working predicate is `eventMessage CONTAINS "deny(1)"` at
# DEFAULT level (adding --level debug actually drops these events). The second
# bug was exiting the whole agent when the stream died (KeepAlive restart left
# a blind gap); here an inner loop restarts the stream in place. The third
# bug was telling EOF from a tick timeout by `read -t`'s status: macOS's
# /bin/bash is 3.2, where both return 1 (bash 4+ returns >128 on timeout), so
# the loop restarted `log stream` every tick and missed most denials. The
# stream now runs behind a FIFO with a known pid, and EOF means that pid is
# gone.
#
# Silence-list: SANDBOX_WATCH_IGNORE_FILE (default sandbox-ignore.conf beside
# this script) holds ERE patterns; a fleet denial matching any is logged
# IGNORED and does not notify. Patterns are matched against the denial message
# with the CMD64 tag stripped ("Sandbox: proc(pid) deny(1) op target") and
# against the summary ("label · proc(pid) · op · target"), so a pattern can
# scope to one label with "^present · ". A second, machine-local list
# (SANDBOX_WATCH_LOCAL_IGNORE_FILE, default ~/.config/sandbox-watch/ignore.conf)
# is checked too. Machine-specific silences go there, NOT beside the script:
# this script ships in a ralph sources cache, where local edits dirty the clone
# and die on the next sync. Both files are re-read for every denial, so edits
# apply without a restart.
# See "Triaging a denial" in recipes/speak/CLAUDE.md.
#
# Generic: one watcher covers every process launched through srt-run, and
# every legacy profile that tags its denies. There is no process list to keep
# in step.
#
# Gotcha: notifications from a launchd agent are attributed to "Script
# Editor" — allow once in System Settings -> Notifications -> Script Editor.
set -u

LOGFILE="${SANDBOX_WATCH_LOG:-$HOME/.local/share/speak/logs/sandbox-denials.log}"
NOTIFYLOG="${SANDBOX_WATCH_NOTIFY_LOG:-$HOME/.local/share/speak/logs/sandbox-notifications.log}"
WINDOW="${SANDBOX_WATCH_WINDOW:-60}"   # notification batch cadence (seconds)
TICK="${SANDBOX_WATCH_TICK:-5}"        # stream read timeout / flush check (seconds)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IGNORE_FILE="${SANDBOX_WATCH_IGNORE_FILE:-$SCRIPT_DIR/sandbox-ignore.conf}"
LOCAL_IGNORE_FILE="${SANDBOX_WATCH_LOCAL_IGNORE_FILE:-$HOME/.config/sandbox-watch/ignore.conf}"

# Regexes live in variables: bash 3.2 (/bin/bash on macOS) only treats the
# right side of =~ as a regex reliably when it is unquoted and expanded.
RE_EVENT_MESSAGE='"eventMessage":"(Sandbox: [^"]*)"'
RE_DENIAL='^Sandbox: ([^ ]+) deny\([0-9]+\) ([a-z*-]+) ?(.*)$'
RE_LABEL="SRT_RUN=['\"]?([A-Za-z0-9._-]+)"
# The legacy marker: the rule's message, which the log appends after an
# escaped newline. The leading greedy .* makes the LAST occurrence win, so a
# denied path that happens to contain the text can't override the marker the
# kernel appended after it.
RE_RAW_MARKER='.*SRT_RUN=([A-Za-z0-9._-]+)'

# The CMD64 tag's base64 payload, decoded. Robust to missing "=" padding and to
# a tag the log cut short (no _END_): the payload stops at the first character
# outside the base64 alphabet and is re-padded. A cut payload can end in a
# partial quantum with stray low bits, which base64 -d rejects outright, so
# that case falls back to decoding only the complete 4-character quanta. The
# label sits at the start of the command, so the lost tail never matters.
decode_cmd64() {
  local b64=${1#*CMD64_} out=""
  b64=${b64%%_END_*}
  b64=${b64%%[!A-Za-z0-9+/=]*}
  b64=${b64%%=*}
  case $(( ${#b64} % 4 )) in
    2) out=$(/usr/bin/base64 -d <<< "$b64==" 2>/dev/null) ;;
    3) out=$(/usr/bin/base64 -d <<< "$b64=" 2>/dev/null) ;;
  esac
  if [ -z "$out" ]; then
    b64=${b64:0:$(( ${#b64} / 4 * 4 ))}
    [ -n "$b64" ] || return 1
    out=$(/usr/bin/base64 -d <<< "$b64" 2>/dev/null)
  fi
  [ -n "$out" ] || return 1
  printf '%s' "$out"
}

# Sets LABEL from a denial message: the SRT_RUN label for a fleet denial, empty
# for a foreign one. Returns 1 when the message carries neither an srt tag nor
# a legacy marker. The srt tag is checked first: the kernel appends it, so
# nothing the confined process controls can stand in for it.
denial_label() {
  local msg=$1 decoded
  LABEL=""
  case "$msg" in
    *CMD64_*) ;;
    *)
      [[ $msg =~ $RE_RAW_MARKER ]] || return 1
      LABEL=${BASH_REMATCH[1]}
      return 0
      ;;
  esac
  decoded=$(decode_cmd64 "$msg") || return 0
  if [[ $decoded =~ $RE_LABEL ]]; then
    LABEL=${BASH_REMATCH[1]}
  fi
  return 0
}

# The denial message without its tag: the log joins srt's tag, or a legacy
# profile's marker, with an escaped newline ("\nCMD64_...", "\nSRT_RUN=..."),
# and an srt tag cut short may lack the newline.
strip_tag() {
  local msg=${1%%\\nCMD64_*}
  msg=${msg%%CMD64_*}
  # Only a line without an srt tag has a legacy marker to cut; on an srt line
  # the text would be part of the denied path.
  if [ "$msg" = "$1" ] && [[ $msg =~ $RE_RAW_MARKER ]]; then
    msg=${msg%SRT_RUN=*}
    msg=${msg%\\n}
  fi
  printf '%s' "$msg"
}

# "Sandbox: present(123) deny(1) file-read-data /path" + label "present"
# -> "present · present(123) · file-read-data · /path"
summarize() {
  local msg=$1 label=$2
  if [[ $msg =~ $RE_DENIAL ]]; then
    printf '%s · %s · %s · %s' "$label" "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}"
  else
    printf '%s · %s' "$label" "$msg"
  fi
}

# Read the active patterns of both ignore files into PATTERNS (comments and
# blank lines skipped). Pure bash, no fork.
load_patterns() {
  local pat f
  PATTERNS=()
  for f in "$IGNORE_FILE" "$LOCAL_IGNORE_FILE"; do
    [ -r "$f" ] || continue
    while IFS= read -r pat || [ -n "$pat" ]; do
      case "$pat" in ''|'#'*) continue ;; esac
      PATTERNS+=("$pat")
    done < "$f"
  done
}

# match (silence) if any line of $1 matches any active ERE in either ignore
# file. One grep for the whole list; if that grep errors (exit 2, one bad
# pattern poisons a combined run) fall back to one grep per pattern, so a typo
# in one line never disables the rest.
is_ignored() {
  local text=$1 pat rc i
  local -a args
  load_patterns
  [ "${#PATTERNS[@]}" -gt 0 ] || return 1
  args=()
  for pat in "${PATTERNS[@]}"; do args+=(-e "$pat"); done
  /usr/bin/grep -Eq "${args[@]}" <<< "$text" 2>/dev/null
  rc=$?
  [ "$rc" -le 1 ] && return "$rc"
  for ((i = 0; i < ${#PATTERNS[@]}; i++)); do
    /usr/bin/grep -Eq -- "${PATTERNS[$i]}" <<< "$text" 2>/dev/null && return 0
  done
  return 1
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

# The events.this payload for the pending ALERT batch. Pulls the label,
# process and operation out of the first summary ("label · proc(pid) ·
# operation · target") so the event is filterable by srt label (the `command`
# tag), process and denial type; message carries the batch's unique denials,
# not just the first. IGNORED denials never reach a batch.
event_payload() {
  local ev_label ev_proc ev_rest ev_op
  ev_label="${batch_first%% · *}"   # "present"
  ev_rest="${batch_first#* · }"     # "present(123) · file-read-data · /path"
  ev_proc="${ev_rest%% · *}"        # "present(123)"
  ev_proc="${ev_proc%%(*}"          # "present"
  ev_rest="${ev_rest#* · }"         # "file-read-data · /path"
  ev_op="${ev_rest%% · *}"          # "file-read-data"
  printf '{"source":"sandbox-watch","level":"warn","title":"%s sandbox denial(s)","message":"%s","tags":{"unique":"%s","command":"%s","process":"%s","op":"%s","status":"alert"}}' \
    "$batch_count" "$(json_escape "${batch_unique:0:2000}")" "$batch_unique_n" \
    "$(json_escape "$ev_label")" "$(json_escape "$ev_proc")" "$(json_escape "$ev_op")"
}

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
    # archived nothing, ever).
    /usr/bin/curl -m 2 -s -o /dev/null -X POST \
      -H 'Content-Type: application/json' \
      --data "$(event_payload)" \
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

# Parse one ndjson stream line and route it: dropped (no srt tag), IGNORED
# (foreign, or a fleet denial an ignore pattern matches) or ALERT.
handle_line() {
  local line=$1 msg summary ts
  case "$line" in *'deny('*) ;; *) return 0 ;; esac
  [[ $line =~ $RE_EVENT_MESSAGE ]] || return 0
  msg=${BASH_REMATCH[1]//\\\//\/}    # JSON "\/" -> "/"
  denial_label "$msg" || return 0   # no CMD64 tag: a system daemon, not ours
  msg=$(strip_tag "$msg")
  ts=$(/bin/date '+%Y-%m-%d %H:%M:%S')
  if [ -z "$LABEL" ]; then
    # srt-tagged but no SRT_RUN marker: an agent's own bash sandbox.
    printf '%s\tIGNORED\t%s\tforeign\n' "$ts" "$(summarize "$msg" -)" >> "$LOGFILE"
    return
  fi
  summary=$(summarize "$msg" "$LABEL")
  if is_ignored "$msg
$summary"; then
    printf '%s\tIGNORED\t%s\tignore-list\n' "$ts" "$summary" >> "$LOGFILE"
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

LOG_BIN="${SANDBOX_WATCH_LOG_BIN:-/usr/bin/log}"   # tests swap in a fake producer
STREAM_PID=""
STREAM_PENDING=""   # start of a line a tick timeout cut in two (bash 4+)

# Start the live stream behind a FIFO so its pid is known, and hand it to
# fd 3. The FIFO is unlinked once both ends are open.
stream_open() {
  local fifo
  fifo="$(/usr/bin/mktemp -u "${TMPDIR:-/tmp}/sandbox-watch.XXXXXX")" || return 1
  /usr/bin/mkfifo -m 600 "$fifo" || return 1
  "$LOG_BIN" stream --style ndjson \
    --predicate 'eventMessage CONTAINS "deny(1)"' > "$fifo" &
  STREAM_PID=$!
  STREAM_PENDING=""
  exec 3< "$fifo"
  rm -f "$fifo"
}

stream_close() {
  exec 3<&-
  if [ -n "$STREAM_PID" ]; then
    kill "$STREAM_PID" 2>/dev/null || true
    wait "$STREAM_PID" 2>/dev/null || true
  fi
  STREAM_PID=""
}

# Read one line from fd 3 into $line, waiting at most TICK seconds. Returns
# 0 on a line, 1 on a tick timeout, 2 when the stream has ended. `read -t`
# alone can't tell the last two apart on bash 3.2 (both exit 1), so EOF is
# the producer being gone. A timeout that lands mid-line leaves the bytes
# read so far in $line on bash 4+ (3.2 drops them); they are kept and
# prepended to the rest of the line on the next read.
stream_read() {
  line=""
  if IFS= read -r -t "$TICK" line <&3; then
    line="$STREAM_PENDING$line"
    STREAM_PENDING=""
    return 0
  fi
  STREAM_PENDING="$STREAM_PENDING$line"
  kill -0 "$STREAM_PID" 2>/dev/null && return 1
  return 2
}

# Outer loop: (re)start the live stream forever. Inner loop: read with a tick
# timeout so batched notifications flush even when the stream is quiet; when
# the stream dies, break to restart it in place — the agent never exits, so
# there's no KeepAlive-restart gap.
main() {
  mkdir -p "$(dirname "$LOGFILE")"
  trap 'stream_close; exit 0' TERM INT
  while :; do
    if ! stream_open; then
      sleep 5
      continue
    fi
    while :; do
      stream_read
      case $? in
        0) handle_line "$line" ;;
        2) break ;;
      esac
      flush
    done
    stream_close
    flush
    sleep 1
  done
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main
fi
