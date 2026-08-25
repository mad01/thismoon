# why opener

## The problem

"Open this" is a constant last-mile move — a PR link into the browser, a
generated file into its app, a directory into Finder — and from an agent
session the only route was a shell call to the macOS open command. That
means a permission prompt for a harmless local action, hand-built shell
strings around paths with spaces, and no discoverability: the agent has to
remember open's flag soup (-a, -R) instead of being offered tools named for
the intent.

## Why its own tool

The fix is a first-class tool surface, not a smarter shell habit. Tools
named open_url, open_file, open_app, open_with, and reveal_in_finder are
discoverable from their descriptions, take structured parameters (no shell
quoting at all), and can validate before launching: a scheme check on URLs,
a ~-expansion, absolute-path check, and stat on files — so a typo fails
with a clear error instead of a GUI dialog. thismoon components are one
binary per concern with a CLI and an MCP frontend over the same core; the
open bridge is exactly such a concern.

## Why this shape

The core is one exec of /usr/bin/open with the right flags per verb. No
serve process and no store — Launch Services owns everything after the
exec returns. The binary path is absolute because MCP hosts spawn processes
with a minimal PATH. macOS-only, matching the platform's darwin/arm64
target (docs/adr/0007).

The component and binary are named **opener**, not open: an installed
binary literally named open would shadow /usr/bin/open on any machine
whose PATH puts personal bins first, silently breaking every script that
calls open. The MCP tool names keep the open_ prefix — they are what agents
see, and they name the intent.

## Non-goals

No windowing control (which display, which space), no waiting for the app
to exit (open -W), and no output capture from the opened app. opener hands
a target to Launch Services and reports what it handed over; what the GUI
does with it is out of scope.
