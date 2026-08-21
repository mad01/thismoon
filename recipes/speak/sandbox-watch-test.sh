#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SANDBOX_WATCH_PROCESSES=present
SANDBOX_WATCH_LOG=/dev/null
SANDBOX_WATCH_NOTIFY_LOG=/dev/null
SANDBOX_WATCH_IGNORE_FILE="$SCRIPT_DIR/sandbox-ignore.conf"
SANDBOX_WATCH_LOCAL_IGNORE_FILE=/dev/null
SANDBOX_WATCH_WINDOW=60
export SANDBOX_WATCH_PROCESSES SANDBOX_WATCH_LOG SANDBOX_WATCH_NOTIFY_LOG
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

# Agent sandbox markers appended after a target must not defeat the boundary.
expect_ignored 'Sandbox: present(123) deny(1) sysctl-read kern.bootargs\nCMD64_test_END__test_SBX'

# Keep the silence list narrow by process, operation, and exact key/service.
expect_alert 'Sandbox: github-mcp-server(123) deny(1) sysctl-read kern.bootargs'
expect_alert 'Sandbox: present(123) deny(1) sysctl-read hw.optional.arm64'
expect_alert 'Sandbox: present(123) deny(1) sysctl-read hw.optional.neon_fp160'
expect_alert 'Sandbox: present(123) deny(1) sysctl-read hw.optional.neon_fp16-extra'
expect_alert 'Sandbox: present(123) deny(1) mach-lookup com.apple.logd2'
expect_alert 'Sandbox: present(123) deny(1) mach-lookup com.apple.logd-helper'
expect_alert 'Sandbox: present(123) deny(1) network-outbound remote:*:443'

# A first alert after a long idle period starts a fresh window instead of
# flushing immediately. The normal flush path must leave this batch pending.
batch_count=0
batch_started=0
before=$(/bin/date +%s)
handle_line '{"eventMessage":"Sandbox: present(123) deny(1) sysctl-read hw.optional.arm64"}'
after=$(/bin/date +%s)

[ "$batch_count" -eq 1 ] || fail "expected one pending alert, got $batch_count"
[ "$batch_started" -ge "$before" ] || fail "batch started before the alert"
[ "$batch_started" -le "$after" ] || fail "batch started after the alert"
flush
[ "$batch_count" -eq 1 ] || fail "fresh batch flushed immediately"

printf 'PASS: sandbox-watch batching and ignore rules\n'
