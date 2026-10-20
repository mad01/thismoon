#!/bin/sh
# Launch `deps serve` for the t-man agent with the mise shims on PATH.
#
# Go discovery runs `go list`, and go comes from mise, whose shims are not on
# the PATH t-man gives the launchd agent. Without them every Go scan fails with
# `exec: "go": executable file not found`. Prepending the shims directory, when
# it exists, lets serve run whatever toolchain mise resolves, and keeps the
# mise location out of the recipe. Every argument passes through unchanged.
set -eu

shims="${MISE_DATA_DIR:-$HOME/.local/share/mise}/shims"
if [ -d "$shims" ]; then
	PATH="$shims:$PATH"
	export PATH
fi

exec "$HOME/code/bin/deps" serve "$@"
