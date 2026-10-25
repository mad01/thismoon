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
# environment; stderr goes to $home/stderr. The runtime is pinned to node,
# so a bun on the caller's PATH can't change what the layer cases test.
launch() {
  env -i HOME="$home" TMPDIR="$home/tmp" PATH="$PATH" SRT_RUN_RUNTIME=node "$@" \
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

# --- runtime: bun or node --------------------------------------------------

# new_mise_home <name>: new_home laid out the way mise installs srt. The srt
# shim links to a stand-in mise, which answers `mise which srt` with npm's
# bin wrapper and, called as srt, runs that wrapper. The wrapper is the
# stand-in srt (the node path); the package's dist/cli.js sits behind a link
# into .mise, as aube lays it out. Sets $cli to the real cli.js.
new_mise_home() {
  new_home "$1"
  local modules="$home/.local/share/mise/installs/npm-srt/1/node_modules"
  local pkg="$modules/.mise/sandbox-runtime/node_modules/@anthropic-ai/sandbox-runtime"
  mkdir -p "$modules/.bin" "$modules/@anthropic-ai" "$pkg/dist" "$home/opt"
  mv "$home/.local/share/mise/shims/srt" "$modules/.bin/srt"
  ln -s "../.mise/sandbox-runtime/node_modules/@anthropic-ai/sandbox-runtime" \
    "$modules/@anthropic-ai/sandbox-runtime"
  cli="$pkg/dist/cli.js"
  printf '// srt\n' > "$cli"
  cat > "$home/opt/mise" << STANDIN
#!/bin/sh
case \${0##*/} in srt) exec "$modules/.bin/srt" "\$@" ;; esac
[ "\$1 \$2" = "which srt" ] || exit 98
printf '%s\n' "$modules/.bin/srt"
STANDIN
  chmod +x "$home/opt/mise"
  ln -s "$home/opt/mise" "$home/.local/share/mise/shims/srt"
}

# fake_bun <path>: a bun stand-in that records its own path, then its argv,
# one per line.
fake_bun() {
  mkdir -p "$(dirname "$1")"
  cat > "$1" << 'STANDIN'
#!/bin/sh
printf '%s\n' "$0" "$@" > "$HOME/bun-argv"
STANDIN
  chmod +x "$1"
}

# launch_rt <PATH> [VAR=value...]: launch with an exact caller PATH and no
# runtime pinned, so only the scratch HOME and that PATH decide.
launch_rt() {
  local path=$1
  shift
  env -i HOME="$home" TMPDIR="$home/tmp" PATH="$path" "$@" \
    sh "$SRT_RUN" "$home/.config/srt/svc.json" true 2> "$home/stderr"
}

# expect_bun <bun> <name>: that bun ran srt's cli.js with the hardening
# flags and srt's usual arguments, and the node wrapper never started.
expect_bun() {
  local argv
  [ -f "$home/bun-argv" ] || fail "$2: bun did not run: $(cat "$home/stderr")"
  [ ! -e "$home/handed-path" ] || fail "$2: the node wrapper ran too"
  [ "$(sed -n 1p "$home/bun-argv")" = "$1" ] ||
    fail "$2: ran $(sed -n 1p "$home/bun-argv"), want $1"
  argv=$(sed -n '2,4p;6,$p' "$home/bun-argv" | tr '\n' ' ')
  [ "$argv" = "--no-env-file --config=/dev/null --no-install --settings $home/.config/srt/svc.json -- env SRT_RUN=svc true " ] ||
    fail "$2: bun argv: $(tr '\n' ' ' < "$home/bun-argv")"
  [ "$(sed -n 5p "$home/bun-argv")" -ef "$cli" ] ||
    fail "$2: bun ran $(sed -n 5p "$home/bun-argv"), not $cli"
  grep -qx 'srt-run: svc runtime bun' "$home/stderr" || fail "$2: no runtime line: $(cat "$home/stderr")"
}

# Unset, a bun in mise's installs dir wins, whatever PATH holds.
new_mise_home bun-installs
fake_bun "$home/.local/share/mise/installs/bun/1.0.0/bin/bun"
fake_bun "$home/pathbin/bun"
launch_rt "$home/pathbin:/usr/bin:/bin" || fail "installs-dir bun exited $?: $(cat "$home/stderr")"
expect_bun "$home/.local/share/mise/installs/bun/1.0.0/bin/bun" "installs-dir bun"

# Unset, with none installed, a bun on PATH is used.
new_mise_home bun-path
fake_bun "$home/pathbin/bun"
launch_rt "$home/pathbin:/usr/bin:/bin" || fail "PATH bun exited $?: $(cat "$home/stderr")"
expect_bun "$home/pathbin/bun" "PATH bun"

# An srt that links straight to dist/cli.js (npm's own layout) works too.
new_home npm-link
mkdir -p "$home/lib/sandbox-runtime/dist" "$home/pathbin"
cli="$home/lib/sandbox-runtime/dist/cli.js"
printf '// srt\n' > "$cli"
chmod +x "$cli"   # npm marks a bin's target executable
rm "$home/.local/share/mise/shims/srt"
ln -s "$cli" "$home/pathbin/srt"
fake_bun "$home/.local/share/mise/installs/bun/1.0.0/bin/bun"
launch_rt "$home/pathbin:/usr/bin:/bin" || fail "linked bin exited $?: $(cat "$home/stderr")"
expect_bun "$home/.local/share/mise/installs/bun/1.0.0/bin/bun" "linked bin"

# SRT_RUN_RUNTIME=node runs the srt wrapper even with bun installed.
new_mise_home node-forced
fake_bun "$home/.local/share/mise/installs/bun/1.0.0/bin/bun"
launch_rt /usr/bin:/bin SRT_RUN_RUNTIME=node || fail "forced node exited $?: $(cat "$home/stderr")"
[ "$(cat "$home/handed-path" 2> /dev/null)" = "$home/.config/srt/svc.json" ] ||
  fail "forced node did not run the srt wrapper: $(cat "$home/stderr")"
[ ! -e "$home/bun-argv" ] || fail "forced node ran bun"
grep -qx 'srt-run: svc runtime node' "$home/stderr" || fail "forced node line: $(cat "$home/stderr")"

# Unset, with no bun anywhere, node is the fallback.
new_mise_home node-fallback
launch_rt /usr/bin:/bin || fail "node fallback exited $?: $(cat "$home/stderr")"
[ -e "$home/handed-path" ] || fail "node fallback did not run the srt wrapper"
grep -qx 'srt-run: svc runtime node' "$home/stderr" || fail "fallback line: $(cat "$home/stderr")"

# SRT_RUN_RUNTIME=bun fails closed without a bun, and without srt's cli.js;
# neither runtime starts. Any other value is a usage error.
new_mise_home bun-missing
status=0
launch_rt /usr/bin:/bin SRT_RUN_RUNTIME=bun || status=$?
[ "$status" -eq 127 ] || fail "bun missing: exited $status, want 127"
[ ! -e "$home/handed-path" ] || fail "bun missing: fell back to node"
grep -qF 'no bun under' "$home/stderr" || fail "bun missing message: $(cat "$home/stderr")"

new_mise_home cli-missing
fake_bun "$home/.local/share/mise/installs/bun/1.0.0/bin/bun"
rm "$cli"
status=0
launch_rt /usr/bin:/bin SRT_RUN_RUNTIME=bun || status=$?
[ "$status" -eq 127 ] || fail "cli missing: exited $status, want 127"
{ [ ! -e "$home/bun-argv" ] && [ ! -e "$home/handed-path" ]; } || fail "cli missing: a runtime started"
grep -qF "cannot find srt's dist/cli.js" "$home/stderr" || fail "cli missing message: $(cat "$home/stderr")"

new_mise_home runtime-bogus
status=0
launch_rt /usr/bin:/bin SRT_RUN_RUNTIME=deno || status=$?
[ "$status" -eq 2 ] || fail "bogus runtime: exited $status, want 2"
[ ! -e "$home/handed-path" ] || fail "bogus runtime: srt started"

printf 'PASS: srt-run merge layers (%d widening, %d refused), runtime bun/node\n' "$oks" "$refusals"
