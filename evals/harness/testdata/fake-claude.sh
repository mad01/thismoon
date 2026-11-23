#!/bin/sh
# fake-claude stands in for the claude CLI in tests. It records its
# arguments, prints a canned stream-json transcript, and then does what the
# FAKE_CLAUDE_* variables ask:
#   FAKE_CLAUDE_STREAM       transcript to print (required)
#   FAKE_CLAUDE_ARGS         file to write the arguments to, one per line
#   FAKE_CLAUDE_TOUCH        file to append a line to, relative to the cwd
#   FAKE_CLAUDE_IGNORE_TERM  ignore SIGTERM, here and in children
#   FAKE_CLAUDE_PIDFILE      start a background sleep and write its pid here
#   FAKE_CLAUDE_SLEEP        seconds to sleep after printing
if [ -n "$FAKE_CLAUDE_ARGS" ]; then printf '%s\n' "$@" > "$FAKE_CLAUDE_ARGS"; fi
if [ -n "$FAKE_CLAUDE_TOUCH" ]; then echo touched >> "$FAKE_CLAUDE_TOUCH"; fi
if [ -n "$FAKE_CLAUDE_IGNORE_TERM" ]; then trap '' TERM; fi
if [ -n "$FAKE_CLAUDE_PIDFILE" ]; then
	sleep 60 </dev/null >/dev/null 2>&1 &
	echo $! > "$FAKE_CLAUDE_PIDFILE"
fi
cat "$FAKE_CLAUDE_STREAM"
if [ -n "$FAKE_CLAUDE_SLEEP" ]; then sleep "$FAKE_CLAUDE_SLEEP"; fi
exit 0
