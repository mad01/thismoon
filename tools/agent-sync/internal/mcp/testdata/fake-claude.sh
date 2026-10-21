#!/bin/sh
# Fake claude CLI for the mcp parity cases. It appends its argv to
# $FAKE_CLAUDE_DIR/argv.log, one call per line with each argument in
# brackets, then replays a canned response from $FAKE_CLAUDE_DIR.
#
# A response is any of <key>.stdout, <key>.stderr and <key>.code (the exit
# status, 0 when absent). The first key with a file wins:
#   call-<n>        the n-th call of the case, counted across its steps
#   <verb>-<arg>    the verb and the argument after it, e.g. get-alpha
#   <verb>          list, get, add, add-json or remove
# With no response, get exits 1 (no such server) and the rest exit 0.
dir="$FAKE_CLAUDE_DIR"
log="$dir/argv.log"
{
	printf 'claude'
	for arg in "$@"; do printf ' [%s]' "$arg"; done
	printf '\n'
} >>"$log"
n=$(wc -l <"$log" | tr -d ' ')
verb="$2"
for key in "call-$n" "$verb-$3" "$verb"; do
	if [ -e "$dir/$key.stdout" ] || [ -e "$dir/$key.stderr" ] || [ -e "$dir/$key.code" ]; then
		[ -e "$dir/$key.stdout" ] && cat "$dir/$key.stdout"
		[ -e "$dir/$key.stderr" ] && cat "$dir/$key.stderr" >&2
		code=0
		[ -e "$dir/$key.code" ] && code=$(cat "$dir/$key.code")
		exit "$code"
	fi
done
if [ "$verb" = get ]; then
	printf 'No MCP server named "%s".\n' "$3"
	exit 1
fi
exit 0
