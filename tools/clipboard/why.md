# why clipboard

## The problem

Handing text to the user is a core last-mile move — a command to run
elsewhere, a link, a snippet for a doc — and the clipboard is where that
handoff happens. From an agent session the only route was a shell call
piping into pbcopy, which triggers a permission prompt for a harmless
operation, invites quoting bugs the moment the text carries backticks or
dollar signs, and is invisible to tool discovery: the agent has to remember
`| pbcopy` rather than being offered a clipboard.

## Why its own tool

The fix is a first-class tool surface, not a smarter shell habit. An MCP
tool named clipboard_copy is discoverable from its description, takes the
text as a structured parameter (no shell quoting at all), and can carry its
own semantics: verbatim copy, a byte count back as confirmation, and a
note explaining an empty paste. thismoon components are one binary per
concern with a CLI and an MCP frontend over the same core; the clipboard is
exactly such a concern, and bolting it onto an unrelated service would bury
it.

## Why this shape

The core is two functions over exec: pbcopy with the text on stdin, pbpaste
with its stdout captured. No serve process and no store — the pasteboard is
the state and macOS owns it. Binary paths are absolute because MCP hosts
and launchd spawn processes with a minimal PATH. macOS-only, matching the
platform's darwin/arm64 target (docs/adr/0007).

## Non-goals

No clipboard history, no images or rich content (pbpaste's text rendering
is the contract), no watching for clipboard changes. A copy is a copy.
