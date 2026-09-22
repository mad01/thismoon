#!/bin/sh
# Launch `present serve` for the t-man agent, with the sharing config.
#
# The Share button in the page view needs PRESENT_SHARED_URL and
# PRESENT_AUTHOR_KEY in the serve process. present-shared-env.sh reads exactly
# those two from the ralph-managed secrets file at start, so the t-man
# definition carries no secret and stays the recipe's `serve --port ...
# --workdir ...`; every argument passes through unchanged. Nothing here is
# sandboxed: serve is first-party code that has always run bare.
set -eu

# shellcheck source=present-shared-env.sh
. "$(dirname "$0")/present-shared-env.sh"

exec "$HOME/code/bin/present" serve "$@"
