#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_TMP="$(mktemp -d)"
trap 'rm -r "$TEST_TMP"' EXIT

SANDBOX_WATCH_LOG="$TEST_TMP/denials.log"
SANDBOX_WATCH_NOTIFY_LOG=/dev/null
SANDBOX_WATCH_IGNORE_FILE="$SCRIPT_DIR/sandbox-ignore.conf"
SANDBOX_WATCH_LOCAL_IGNORE_FILE=/dev/null
SANDBOX_WATCH_WINDOW=60
export SANDBOX_WATCH_LOG SANDBOX_WATCH_NOTIFY_LOG
export SANDBOX_WATCH_IGNORE_FILE SANDBOX_WATCH_LOCAL_IGNORE_FILE
export SANDBOX_WATCH_WINDOW

# shellcheck source=recipes/speak/sandbox-watch.sh
source "$SCRIPT_DIR/sandbox-watch.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

expect_ignored() {
  is_ignored "$1" || fail "expected ignored: $1"
}

expect_alert() {
  ! is_ignored "$1" || fail "expected alert: $1"
}

expect_eq() {
  [ "$2" = "$3" ] || fail "$1: expected '$3', got '$2'"
}

reset_batch() {
  batch_count=0
  batch_first=""
  batch_unique=""
  batch_unique_n=0
  batch_started=0
  : > "$SANDBOX_WATCH_LOG"
}

# base64 of a command, the way srt encodes it into the CMD64 tag.
b64() {
  printf '%s' "$1" | /usr/bin/base64 | tr -d '\n'   # GNU base64 wraps at 76
}

# One ndjson stream line, JSON-escaped the way `log stream` writes it: "/"
# as "\/" and the tag after a literal "\n". $1 is the denial, $2 the command
# srt ran.
ndjson() {
  local tag
  tag="CMD64_$(b64 "$2")_END__0dkawpeh7_SBX"
  printf '{"eventMessage":"Sandbox: %s\\n%s"}' "${1//\//\\/}" "${tag//\//\\/}"
}

FLEET_CMD='env SRT_RUN=present /Users/x/code/bin/present mcp'
AGENT_CMD="/bin/bash -c 'cat /Users/x/.ssh/id_ed25519'"

# --- CMD64 decoding -----------------------------------------------------

# Padded payloads (one and two "=") decode to the command.
[[ $(b64 'env SRT_RUN=a') == *== ]] || fail "fixture should carry == padding"
[[ $(b64 'env SRT_RUN=ab') == *[!=]= ]] || fail "fixture should carry one = of padding"
expect_eq "padded ==" "$(decode_cmd64 "x\\nCMD64_$(b64 'env SRT_RUN=a')_END__s_SBX")" 'env SRT_RUN=a'
expect_eq "padded =" "$(decode_cmd64 "x\\nCMD64_$(b64 'env SRT_RUN=ab')_END__s_SBX")" 'env SRT_RUN=ab'

# Missing padding is restored.
nopad=$(b64 'env SRT_RUN=a')
expect_eq "unpadded" "$(decode_cmd64 "CMD64_${nopad%%=*}_END__s_SBX")" 'env SRT_RUN=a'

# A tag the log cut short (no _END_, payload ends mid-quantum) still yields
# the decodable prefix, which holds the label.
full=$(b64 "$FLEET_CMD")
[[ $(decode_cmd64 "Sandbox: x\\nCMD64_${full:0:30}") == 'env SRT_RUN=present'* ]] ||
  fail "truncated tag lost the label"
[[ $(decode_cmd64 "Sandbox: x\\nCMD64_${full:0:29}") == 'env SRT_RUN=present'* ]] ||
  fail "truncated tag with a 1-char tail lost the label"

# --- attribution --------------------------------------------------------

denial_label "Sandbox: present(1) deny(1) op t\\nCMD64_$(b64 "$FLEET_CMD")_END__s_SBX" ||
  fail "fleet denial reported as untagged"
expect_eq "fleet label" "$LABEL" present

denial_label "Sandbox: bash(1) deny(1) op t\\nCMD64_$(b64 "$AGENT_CMD")_END__s_SBX" ||
  fail "agent-shell denial reported as untagged"
expect_eq "foreign label" "$LABEL" ""

# A quoted marker (srt quotes words with shell-special characters) still counts.
denial_label "Sandbox: x(1) deny(1) op t\\nCMD64_$(b64 "env 'SRT_RUN=github-mcp' x")_END__s_SBX"
expect_eq "quoted label" "$LABEL" github-mcp

! denial_label 'Sandbox: mds(1) deny(1) file-read-data /x' ||
  fail "untagged system denial treated as srt"

# A legacy seatbelt profile (t-man --sandbox-profile) tags its deny rules with
# (with message "SRT_RUN=<label>"); the log appends that after "\n".
denial_label 'Sandbox: cat(75284) deny(1) file-read-data /private/etc/hosts\nSRT_RUN=probe-legacy' ||
  fail "legacy marker not recognised"
expect_eq "legacy label" "$LABEL" probe-legacy

# The marker the kernel appended last wins over one a denied path carries.
denial_label 'Sandbox: python3(9) deny(1) file-read-data /x/\nSRT_RUN=evil/y\nSRT_RUN=speak-tts' ||
  fail "legacy marker after a spoofed path not recognised"
expect_eq "last marker wins" "$LABEL" speak-tts
expect_eq "strip keeps the path" \
  "$(strip_tag 'Sandbox: python3(9) deny(1) file-read-data /x/\nSRT_RUN=evil/y\nSRT_RUN=speak-tts')" \
  'Sandbox: python3(9) deny(1) file-read-data /x/\nSRT_RUN=evil/y'

expect_eq "strip tag" "$(strip_tag 'Sandbox: a(1) deny(1) op /t\nCMD64_eA==_END__s_SBX')" \
  'Sandbox: a(1) deny(1) op /t'
expect_eq "strip legacy marker" "$(strip_tag 'Sandbox: a(1) deny(1) op /t\nSRT_RUN=speak-tts')" \
  'Sandbox: a(1) deny(1) op /t'
expect_eq "summary" "$(summarize 'Sandbox: present(123) deny(1) file-read-data /p q' present)" \
  'present · present(123) · file-read-data · /p q'

# --- ignore list --------------------------------------------------------

for key in \
  kern.iossupportversion \
  kern.bootargs \
  security.mac.lockdown_mode_state \
  hw.optional.neon_fp16 \
  hw.optional.neon_hpfp; do
  expect_ignored "Sandbox: present(123) deny(1) sysctl-read $key"
done

for service in \
  com.apple.system.notification_center \
  com.apple.logd \
  com.apple.diagnosticd; do
  expect_ignored "Sandbox: present(123) deny(1) mach-lookup $service"
done

# srt's own base profile denies this read from bash at every launch, and its
# launch bash registers with diagnosticd.
expect_ignored 'Sandbox: bash(123) deny(1) sysctl-read kern.iossupportversion'
expect_ignored 'Sandbox: bash(123) deny(1) mach-lookup com.apple.diagnosticd'
expect_alert 'Sandbox: bash(123) deny(1) mach-lookup com.apple.diagnosticd2'
expect_alert 'Sandbox: bash(123) deny(1) mach-lookup com.apple.securityd'
expect_ignored 'Sandbox: node(123) deny(1) mach-lookup com.apple.diagnosticd'
expect_alert 'Sandbox: node(123) deny(1) mach-lookup com.apple.logd'

# Go's resolver under srt: the three denied steps of resolving the proxy's
# localhost are silenced for any process; a connect to a real host is not.
expect_ignored 'Sandbox: github-mcp-server(123) deny(1) network-outbound /private/var/run/mDNSResponder'
expect_ignored 'Sandbox: github-mcp-server(123) deny(1) network-outbound'
expect_ignored 'Sandbox: github-mcp-server(123) deny(1) network-bind local:*:0'
expect_alert 'Sandbox: github-mcp-server(123) deny(1) network-outbound 140.82.121.3:443'
expect_alert 'Sandbox: github-mcp-server(123) deny(1) network-outbound /private/var/run/other.sock'
expect_alert 'Sandbox: github-mcp-server(123) deny(1) network-bind local:*:8080'

# A tag still attached after the target must not defeat the boundary.
expect_ignored 'Sandbox: present(123) deny(1) sysctl-read kern.bootargs\nCMD64_dGVzdA_END__test_SBX'

# Keep the silence list narrow by process, operation, and exact key/service.
expect_alert 'Sandbox: github-mcp-server(123) deny(1) sysctl-read kern.bootargs'
expect_alert 'Sandbox: present(123) deny(1) sysctl-read hw.optional.arm64'
expect_alert 'Sandbox: present(123) deny(1) sysctl-read hw.optional.neon_fp160'
expect_alert 'Sandbox: present(123) deny(1) sysctl-read hw.optional.neon_fp16-extra'
expect_alert 'Sandbox: present(123) deny(1) mach-lookup com.apple.logd2'
expect_alert 'Sandbox: present(123) deny(1) mach-lookup com.apple.logd-helper'
expect_alert 'Sandbox: present(123) deny(1) network-outbound remote:*:443'

# Startup sysctl reads from shared code (libSystem, Go's CPU detection) are
# silenced for any process.
for key in \
  kern.iossupportversion \
  hw.optional.neon_fp16 \
  hw.optional.neon_hpfp \
  hw.optional.AdvSIMD_HPFPCvt \
  hw.optional.armv8_1_atomics \
  hw.optional.armv8_2_sha512 \
  hw.optional.arm.FEAT_SHA3 \
  hw.optional.arm.FEAT_SME_F64F64; do
  expect_ignored "Sandbox: humanizer(18966) deny(1) sysctl-read $key"
  expect_ignored "Sandbox: vale(18968) deny(1) sysctl-read $key"
done

# The any-process entries stay narrow by operation and exact key.
expect_alert 'Sandbox: humanizer(18966) deny(1) sysctl-read hw.optional.arm64'
expect_alert 'Sandbox: humanizer(18966) deny(1) sysctl-read hw.model'
expect_alert 'Sandbox: humanizer(18966) deny(1) sysctl-read hw.optional.arm.caps'
expect_alert 'Sandbox: humanizer(18966) deny(1) sysctl-read kern.bootargs'
expect_alert 'Sandbox: humanizer(18966) deny(1) sysctl-read kern.iossupportversion2'
expect_alert 'Sandbox: humanizer(18966) deny(1) sysctl-read hw.optional.neon_fp16-extra'
expect_alert 'Sandbox: humanizer(18966) deny(1) sysctl-write kern.iossupportversion'
expect_alert 'Sandbox: humanizer(18966) deny(1) file-read-data /Users/x/kern.iossupportversion'

# Go's listen-backlog sysctl is silenced for any process, by exact key.
expect_ignored 'Sandbox: moondb(123) deny(1) sysctl-read kern.ipc.somaxconn'
expect_alert 'Sandbox: moondb(123) deny(1) sysctl-read kern.ipc.somaxconn2'
expect_alert 'Sandbox: moondb(123) deny(1) sysctl-read kern.ipc.maxsockbuf'

# The launch bash stats a t-man service's own std logs; nothing wider.
expect_ignored 'Sandbox: bash(123) deny(1) file-read-metadata /Users/x/Library/Logs/events/stdout.log'
expect_ignored 'Sandbox: bash(123) deny(1) file-read-metadata /Users/x/Library/Logs/keeper-of-facts/stderr.log'
expect_alert 'Sandbox: bash(123) deny(1) file-read-data /Users/x/Library/Logs/events/stdout.log'
expect_alert 'Sandbox: events(123) deny(1) file-read-metadata /Users/x/Library/Logs/events/stdout.log'
expect_alert 'Sandbox: bash(123) deny(1) file-read-metadata /Users/x/Library/Logs/events/other.log'
expect_alert 'Sandbox: bash(123) deny(1) file-read-metadata /Users/x/Library/Logs/a/b/stdout.log'
expect_alert 'Sandbox: bash(123) deny(1) file-read-metadata /Users/x/.ssh/stdout.log'

# clang++ (go list's C++ compiler check under deps) is a process name too.
expect_ignored 'Sandbox: clang++(123) deny(1) sysctl-read kern.iossupportversion'
expect_ignored 'Sandbox: clang++(123) deny(1) mach-lookup com.apple.diagnosticd'
expect_alert 'Sandbox: clang++(123) deny(1) file-read-metadata /Users/x/Downloads/Xcode.app/Contents/Developer/usr/lib/libxcrun.dylib'

# deps: the Go toolchain's telemetry counters, by label, tool and directory.
for proc in go compile asm link cgo; do
  expect_ignored "deps · $proc(123) · file-write-data · /Users/x/Library/Application Support/go/telemetry/local/weekends"
done
expect_ignored 'deps · go(123) · file-read-metadata · /Users/x/Library/Application Support/go/telemetry/local/upload.token'
expect_alert 'status · go(123) · file-write-data · /Users/x/Library/Application Support/go/telemetry/local/weekends'
expect_alert 'deps · deps(123) · file-write-data · /Users/x/Library/Application Support/go/telemetry/local/weekends'
expect_alert 'deps · go(123) · file-write-data · /Users/x/Library/Application Support/go/telemetry/mode'
expect_alert 'deps · go(123) · file-read-data · /Users/x/Library/Caches/go-build/00/00ab-d'

# deps: osascript's AppKit start-up around the advisory banner, by exact
# service, key and file.
for service in windowserver.active tccd.system pasteboard.1 dock.server \
  CoreServices.coreservicesd analyticsd; do
  expect_ignored "deps · osascript(123) · mach-lookup · com.apple.$service"
done
expect_ignored 'deps · osascript(123) · mach-lookup · com.apple.hiservices-xpcservice (xpc)'
expect_ignored 'deps · osascript(123) · mach-register · com.apple.axserver (per-pid)'
expect_ignored 'deps · osascript(123) · sysctl-read · kern.safeboot'
expect_ignored 'deps · osascript(123) · file-read-data · /Users/x/Library/Preferences/.GlobalPreferences.plist'
expect_ignored 'deps · osascript(123) · file-read-data · /Users/x/Library/Preferences/ByHost/.GlobalPreferences.21972D70-0261-5271-9BC0-F369D8992BFB.plist'
expect_alert 'deps · osascript(123) · mach-lookup · com.apple.securityd'
expect_alert 'deps · osascript(123) · mach-lookup · com.apple.pasteboard.10'
expect_alert 'deps · osascript(123) · file-read-data · /Users/x/Library/Preferences/com.apple.Safari.plist'
expect_alert 'deps · osascript(123) · file-read-data · /Users/x/Library/Keychains/login.keychain-db'
expect_alert 'present · osascript(123) · mach-lookup · com.apple.windowserver.active'
expect_ignored 'status · osascript(123) · mach-lookup · com.apple.windowserver.active'
expect_ignored 'status · osascript(123) · sysctl-read · kern.safeboot'

# status: launchctl's start-up sysctl, by label, process and key.
expect_ignored 'status · launchctl(123) · sysctl-read · kern.singleuser'
expect_alert 'status · launchctl(123) · sysctl-read · kern.singleuser2'
expect_alert 'deps · launchctl(123) · sysctl-read · kern.singleuser'
expect_alert 'status · status(123) · sysctl-read · kern.singleuser'

# speak-web: the Kokoro voice listing at start, by label, binary and directory.
expect_ignored 'speak-web · speak(123) · file-read-data · /Users/x/.cache/huggingface/hub/models--mlx-community--Kokoro-82M-bf16/snapshots'
expect_alert 'speak-web · speak(123) · file-read-data · /Users/x/.cache/huggingface/token'
expect_alert 'speak-web · speak(123) · file-read-data · /Users/x/.cache/huggingface/hub/models--a--b/snapshots/c/voices/af_heart.safetensors'
expect_alert 'speak-web · speak(123) · file-read-data · /Users/x/.config/ralph/secrets.sh'
expect_alert 'present · speak(123) · file-read-data · /Users/x/.cache/huggingface/hub/models--a--b/snapshots'

# A machine-local pattern can scope by label through the summary line, and a
# broken pattern beside it does not disable the rest of the list.
printf '%s\n' '# comment' '' '^humanizer · .* · file-read-data · /Users/[^/]+/\.cache/' '(' \
  > "$TEST_TMP/local.conf"
LOCAL_IGNORE_FILE="$TEST_TMP/local.conf"
expect_ignored 'Sandbox: vale(1) deny(1) file-read-data /Users/x/.cache/v
humanizer · vale(1) · file-read-data · /Users/x/.cache/v'
expect_alert 'Sandbox: vale(1) deny(1) file-read-data /Users/x/.cache/v
present · vale(1) · file-read-data · /Users/x/.cache/v'
expect_ignored 'Sandbox: x(1) deny(1) file-write-data /dev/dtracehelper'
LOCAL_IGNORE_FILE=/dev/null

# --- end to end through handle_line -------------------------------------

# Launch noise from a fleet process adds no alert, a foreign denial and an
# untagged system denial add none either, and a file write from the fleet
# process right after them does.
HUMANIZER_CMD='env SRT_RUN=humanizer /Users/x/code/bin/humanizer mcp'
reset_batch
for denial in \
  'bash(18960) deny(1) sysctl-read kern.iossupportversion' \
  'humanizer(18966) deny(1) sysctl-read hw.optional.neon_fp16' \
  'humanizer(18966) deny(1) sysctl-read hw.optional.neon_hpfp' \
  'vale(18968) deny(1) sysctl-read kern.iossupportversion'; do
  handle_line "$(ndjson "$denial" "$HUMANIZER_CMD")"
done
handle_line "$(ndjson 'bash(7) deny(1) file-read-data /Users/x/.ssh/id_ed25519' "$AGENT_CMD")"
# A foreign line exactly as the log wrote it for an agent's bash sandbox.
handle_line '{"eventMessage":"Sandbox: bash(65561) deny(1) file-write-create \/private\/tmp\/srt-probe\/denied.txt\nCMD64_c2ggLWMgJ2NhdCAkSE9NRS8uc3NoL2tub3duX2hvc3RzID4gL3RtcC9zcnQtcHJvYmUvb3V0L2sudHh0OyBlY2hvIHggPiAvdG1wL3NydC1wcm9iZS9kZW5pZWQudHh0Jw==_END__0dkawpeh7_SBX"}'
handle_line '{"eventMessage":"Sandbox: mds(8) deny(1) file-read-data \/Users\/x\/.ssh\/id_ed25519"}'
handle_line '{"eventMessage":"3 duplicate reports for Sandbox: present(9) deny(1) file-read-data \/Users\/x\/.ssh\/id\nSRT_RUN=present"}'
[ "$batch_count" -eq 0 ] || fail "noise raised $batch_count alert(s)"
handle_line "$(ndjson 'humanizer(18966) deny(1) file-write-create /Users/x/.config/humanizer/state' "$HUMANIZER_CMD")"
[ "$batch_count" -eq 1 ] || fail "file-write-create after the launch noise did not alert"
expect_eq "batch first" "$batch_first" \
  'humanizer · humanizer(18966) · file-write-create · /Users/x/.config/humanizer/state'

# The ledger: four ignore-list lines, two foreign lines, nothing for the
# untagged system denial or the duplicate-report summary, one ALERT.
# Columns: ts, status, summary, reason.
ledger=$(cut -f2- "$SANDBOX_WATCH_LOG")
expect_eq "ledger" "$ledger" "IGNORED	humanizer · bash(18960) · sysctl-read · kern.iossupportversion	ignore-list
IGNORED	humanizer · humanizer(18966) · sysctl-read · hw.optional.neon_fp16	ignore-list
IGNORED	humanizer · humanizer(18966) · sysctl-read · hw.optional.neon_hpfp	ignore-list
IGNORED	humanizer · vale(18968) · sysctl-read · kern.iossupportversion	ignore-list
IGNORED	- · bash(7) · file-read-data · /Users/x/.ssh/id_ed25519	foreign
IGNORED	- · bash(65561) · file-write-create · /private/tmp/srt-probe/denied.txt	foreign
ALERT	humanizer · humanizer(18966) · file-write-create · /Users/x/.config/humanizer/state"

# The events payload carries the label as the `command` tag, beside the
# fields it always had.
payload=$(event_payload)
for field in \
  '"source":"sandbox-watch"' \
  '"level":"warn"' \
  '"title":"1 sandbox denial(s)"' \
  '"command":"humanizer"' \
  '"process":"humanizer"' \
  '"op":"file-write-create"' \
  '"unique":"1"' \
  '"status":"alert"'; do
  [[ $payload == *"$field"* ]] || fail "payload lacks $field: $payload"
done

# A legacy seatbelt service's denial, exactly as the log writes it, alerts
# under its label; its launch noise is silenced like any srt process's.
handle_line '{"eventMessage":"Sandbox: python3.14(4242) deny(1) file-read-data \/Users\/x\/.aws\/credentials\nSRT_RUN=speak-tts"}'
handle_line '{"eventMessage":"Sandbox: python3.14(4242) deny(1) sysctl-read kern.iossupportversion\nSRT_RUN=speak-tts"}'
[ "$batch_count" -eq 2 ] || fail "legacy denial did not alert"
expect_eq "legacy ledger" "$(tail -2 "$SANDBOX_WATCH_LOG" | cut -f2-)" "ALERT	speak-tts · python3.14(4242) · file-read-data · /Users/x/.aws/credentials
IGNORED	speak-tts · python3.14(4242) · sysctl-read · kern.iossupportversion	ignore-list"
reset_batch
handle_line "$(ndjson 'humanizer(18966) deny(1) file-write-create /Users/x/.config/humanizer/state' "$HUMANIZER_CMD")"

# Repeats count toward the batch but not toward the unique list.
handle_line "$(ndjson 'humanizer(18966) deny(1) file-write-create /Users/x/.config/humanizer/state' "$HUMANIZER_CMD")"
[ "$batch_count" -eq 2 ] || fail "repeat not counted"
[ "$batch_unique_n" -eq 1 ] || fail "repeat counted as unique"
reset_batch

# A first alert after a long idle period starts a fresh window instead of
# flushing immediately. The normal flush path must leave this batch pending.
before=$(/bin/date +%s)
handle_line "$(ndjson 'present(123) deny(1) sysctl-read hw.optional.arm64' "$FLEET_CMD")"
after=$(/bin/date +%s)

[ "$batch_count" -eq 1 ] || fail "expected one pending alert, got $batch_count"
[ "$batch_started" -ge "$before" ] || fail "batch started before the alert"
[ "$batch_started" -le "$after" ] || fail "batch started after the alert"
flush
[ "$batch_count" -eq 1 ] || fail "fresh batch flushed immediately"

# The stream loop must tell a quiet tick from the stream ending. On bash 3.2
# (macOS /bin/bash) `read -t` exits 1 for both, which once restarted
# `log stream` every tick. A fake producer writes a line, stays quiet past
# one tick, writes another, and exits: expect line, timeout(s), line, EOF.
cat > "$TEST_TMP/fake-log" <<'EOF'
#!/bin/bash
printf 'first\n'
sleep 1.5
printf 'second\n'
EOF
chmod +x "$TEST_TMP/fake-log"
LOG_BIN="$TEST_TMP/fake-log"
TICK=1
stream_open || fail "stream_open failed"
seq=""
for _ in 1 2 3 4 5 6 7 8; do
  rc=0
  stream_read || rc=$?
  case $rc in
    0) seq="$seq,$line" ;;
    1) seq="$seq,tick" ;;
    2) seq="$seq,eof"; break ;;
  esac
done
stream_close
re='^,first(,tick)+,second(,tick)*,eof$'
[[ $seq =~ $re ]] || fail "stream loop saw $seq (expected first, tick, second, eof)"

# A tick that fires mid-line must not split the line. bash 4+ hands the
# partial read back in the variable and stream_read keeps it; bash 3.2
# discards it, a known gap this test does not assert on.
if [ "${BASH_VERSINFO[0]}" -ge 4 ]; then
  cat > "$TEST_TMP/fake-log" <<'EOF'
#!/bin/bash
printf 'sec'
sleep 1.5
printf 'ond\n'
EOF
  stream_open || fail "stream_open failed"
  seq=""
  for _ in 1 2 3 4 5 6; do
    rc=0
    stream_read || rc=$?
    case $rc in
      0) seq="$seq,$line" ;;
      1) seq="$seq,tick" ;;
      2) seq="$seq,eof"; break ;;
    esac
  done
  stream_close
  re='^(,tick)+,second(,tick)*,eof$'
  [[ $seq =~ $re ]] || fail "split line saw $seq (expected tick, second, eof)"
fi

printf 'PASS: sandbox-watch attribution, batching and ignore rules, stream loop\n'
