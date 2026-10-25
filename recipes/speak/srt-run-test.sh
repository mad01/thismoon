#!/bin/bash
# Exercises srt-run's merge layers against a scratch HOME, with a stand-in
# srt on the mise shims path that records the settings file it was handed
# and runs nothing. Covers the expansion layer ~/.config/srt/<label>.local.json
# and SRT_RUN_ALLOW_DOMAINS: widening merges, and anything else refuses the
# launch before srt starts. The layer cases live in testdata/srt-local, which
# srtspec's explain tests run too, so the two rule sets cannot drift apart.
# Bash 3.2 compatible: the macOS system bash runs it as well as CI's.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRT_RUN="$ROOT/recipes/speak/srt-run.sh"
FIXTURES="$ROOT/recipes/speak/testdata/srt-local"

scratch=$(mktemp -d "${TMPDIR:-/tmp}/srt-run-test.XXXXXX")
trap 'rm -r "$scratch"' EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

# new_home <name>: a fresh HOME holding the stand-in srt and base.json as the
# settings for label svc. Sets $home.
new_home() {
  home="$scratch/$1"
  mkdir -p "$home/.local/share/mise/shims" "$home/.config/srt" "$home/tmp"
  cat > "$home/.local/share/mise/shims/srt" << 'STANDIN'
#!/bin/sh
[ "$1" = --settings ] || exit 99
printf '%s\n' "$2" > "$HOME/handed-path"
cp "$2" "$HOME/handed.json"
STANDIN
  chmod +x "$home/.local/share/mise/shims/srt"
  cp "$FIXTURES/base.json" "$home/.config/srt/svc.json"
}

# launch [VAR=value...]: run srt-run for svc under $home with a clean
# environment; stderr goes to $home/stderr.
launch() {
  env -i HOME="$home" TMPDIR="$home/tmp" PATH="$PATH" "$@" \
    sh "$SRT_RUN" "$home/.config/srt/svc.json" true 2> "$home/stderr"
}

# No layer: srt reads the settings file as is, and nothing is merged.
new_home plain
launch || fail "plain launch exited $?: $(cat "$home/stderr")"
[ "$(cat "$home/handed-path")" = "$home/.config/srt/svc.json" ] ||
  fail "plain launch did not hand srt the settings file"
[ ! -e "$home/.config/srt/merged" ] || fail "plain launch wrote a merged copy"

# Widening: the layer and SRT_RUN_ALLOW_DOMAINS both land in the merged copy,
# and every base deny survives.
new_home widen
cp "$FIXTURES/ok-widen.local.json" "$home/.config/srt/svc.local.json"
launch SRT_RUN_ALLOW_DOMAINS=share.example.net,api.example.com ||
  fail "widening launch exited $?: $(cat "$home/stderr")"
merged="$home/.config/srt/merged/svc.json"
[ "$(cat "$home/handed-path")" = "$merged" ] || fail "srt was not handed the merged copy"
[ -n "$(find "$merged" -perm 600)" ] || fail "merged copy is not mode 600"
jq -e '
  .network.allowedDomains == ["*.example.org", "api.example.com", "share.example.net"] and
  .network.allowMachLookup == ["com.apple.SystemConfiguration.configd"] and
  .network.allowUnixSockets == ["/private/var/run/svc.sock"] and
  .network.allowLocalBinding == true and
  .enableWeakerNetworkIsolation == true and
  (.filesystem.allowRead | index("~/code") and index("~/notes")) and
  (.filesystem.allowWrite | index("~/.cache/svc") and index("~/.local/share/svc")) and
  .filesystem.denyRead == ["~", "~/code/**/.env"] and
  .filesystem.denyWrite == []
' "$merged" > /dev/null || fail "merged copy is wrong: $(jq -c . "$merged")"

# Every ok case launches, and keeps the base denies.
shopt -s nullglob
oks=0
for case_file in "$FIXTURES"/ok-*.local.json; do
  name=$(basename "$case_file" .local.json)
  new_home "$name"
  cp "$case_file" "$home/.config/srt/svc.local.json"
  launch || fail "$name: launch exited $?: $(cat "$home/stderr")"
  jq -e --slurpfile base "$FIXTURES/base.json" \
    '.filesystem.denyRead == $base[0].filesystem.denyRead' \
    "$home/.config/srt/merged/svc.json" > /dev/null || fail "$name: a base deny changed"
  oks=$((oks + 1))
done

# Every refuse case exits 1 with a reason, and srt never starts.
refusals=0
for case_file in "$FIXTURES"/refuse-*.local.json; do
  name=$(basename "$case_file" .local.json)
  new_home "$name"
  cp "$case_file" "$home/.config/srt/svc.local.json"
  status=0
  launch || status=$?
  [ "$status" -eq 1 ] || fail "$name: exited $status, want 1"
  [ ! -e "$home/handed-path" ] || fail "$name: srt started despite the refusal"
  grep -q '^srt-run: ' "$home/stderr" || fail "$name: no srt-run message: $(cat "$home/stderr")"
  refusals=$((refusals + 1))
done
{ [ "$oks" -gt 0 ] && [ "$refusals" -gt 0 ]; } || fail "no fixtures under $FIXTURES"

# The refusal names what was wrong.
new_home reasons
cp "$FIXTURES/refuse-deny-key.local.json" "$home/.config/srt/svc.local.json"
launch || true
grep -qF 'filesystem.denyRead is not allowed' "$home/stderr" ||
  fail "deny removal message: $(cat "$home/stderr")"
cp "$FIXTURES/refuse-star.local.json" "$home/.config/srt/svc.local.json"
launch || true
grep -qF 'which allows every host' "$home/stderr" || fail "star message: $(cat "$home/stderr")"
cp "$FIXTURES/refuse-unknown-key.local.json" "$home/.config/srt/svc.local.json"
launch || true
grep -qF 'key allowPty is not allowed' "$home/stderr" ||
  fail "unknown key message: $(cat "$home/stderr")"

# A bare * in SRT_RUN_ALLOW_DOMAINS is still refused without any layer.
new_home envstar
status=0
launch SRT_RUN_ALLOW_DOMAINS='ok.example.com,*:443' || status=$?
{ [ "$status" -eq 1 ] && [ ! -e "$home/handed-path" ]; } || fail "env * was not refused"

printf 'PASS: srt-run merge layers (%d widening, %d refused)\n' "$oks" "$refusals"
