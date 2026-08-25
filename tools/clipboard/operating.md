# operating clipboard

clipboard bridges the macOS system pasteboard to agents and scripts: copy
puts text on the clipboard, paste reads it back. One binary, two frontends —
a CLI and an MCP stdio server — over the same two pasteboard commands.

## how it runs

There is no serve process and no store. `{{.Bin}} mcp` is a stdio process
the MCP host spawns; every tool call execs /usr/bin/pbcopy or
/usr/bin/pbpaste directly, exactly as the CLI commands do. Nothing has to
stay running, and nothing persists between calls — the pasteboard itself is
the only state, and macOS owns it.

## failure modes

Copy or paste fails naming /usr/bin/pbcopy or /usr/bin/pbpaste: the
pasteboard binaries live there on every macOS install, so that error means
the tool is running somewhere that is not macOS. clipboard is macOS-only by
design.

Paste returns empty text: usually not a failure. pbpaste renders text
content only, so an image, a file reference, or a genuinely empty clipboard
all come back as an empty string. The MCP tool marks the case with a note
field; in a shell, `pbpaste | xxd | head` shows whether any bytes exist at
all.

MCP tools fail while the CLI works: the MCP host runs whatever binary was
registered, separately from the one on PATH. On macOS an adhoc-signed binary
whose provenance drifted (a manual copy over the installed file) is killed
on exec, which surfaces as the MCP server dying at startup. Reinstall from
the thismoon checkout with `make install` in tools/clipboard (it strips
xattrs and re-signs), then restart the MCP host.

Copy succeeded but another app still sees old contents: clipboard managers
and Universal Clipboard sync with a delay, and some apps cache the
pasteboard. `{{.Bin}} paste` reads the live pasteboard — if it shows the
new text, the copy landed and the reader is behind.

## version skew

There is no serve process, so skew is between binaries: the MCP host keeps
running the `{{.Bin}} mcp` process it spawned, while `make install`
replaces the binary on PATH. `{{.Bin}} version -o json` reports the
installed build; restart the MCP host (or the session) to pick it up.

## first moves

1. `{{.Bin}} paste` (confirms the binary runs and can read the pasteboard)
2. `echo probe | {{.Bin}} copy && {{.Bin}} paste` (round-trip; overwrites
   whatever the user had copied, so ask first)
3. `{{.Bin}} version -o json` after an install, then restart the MCP host
