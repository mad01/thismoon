#!/bin/sh
# Stand-in for the claude CLI in the mods cases: it appends its argv to
# $FAKE_CLAUDE_LOG and exits with $FAKE_CLAUDE_EXIT (default 0), printing a
# line on stderr when that is not 0. It never touches Claude Code's state.
printf '%s\n' "$*" >> "$FAKE_CLAUDE_LOG"
if [ "${FAKE_CLAUDE_EXIT:-0}" != 0 ]; then
	echo "fake claude: plugin $2 failed" >&2
fi
exit "${FAKE_CLAUDE_EXIT:-0}"
