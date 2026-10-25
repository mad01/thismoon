#!/bin/sh
# Launch `deps serve` for the t-man agent, inside its sandbox
# (anthropics/sandbox-runtime, launched through srt-run).
#
# Go discovery runs `go list`, and go comes from mise. The mise shim reads
# ~/.config/mise on every call, which the sandbox denies, so the real binary
# is resolved here, outside the sandbox, with `mise which go` (or the first
# go on PATH when mise has none). Inside the sandbox, /usr/bin/env puts its
# directory first on PATH: srt-run prepends the mise shims ahead of the
# caller's PATH, so exporting PATH here would not make this go win. Without
# a go, every Go scan fails and serve saves the failure as a discovery error.
#
# The settings are ~/.config/srt/deps.json, which the deps recipe links from
# deps.srt.json. Every argument passes through to `deps serve` unchanged.
set -eu

child_path=/usr/bin:/bin:/usr/sbin:/sbin
gobin=$(mise which go 2> /dev/null || command -v go || true)
if [ -n "$gobin" ]; then
	child_path="$(dirname "$gobin"):$child_path"
fi

cd /
exec "$HOME/code/bin/srt-run" "$HOME/.config/srt/deps.json" \
	/usr/bin/env PATH="$child_path" "$HOME/code/bin/deps" serve "$@"
