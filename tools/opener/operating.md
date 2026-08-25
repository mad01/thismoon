# operating opener

opener bridges the macOS open command to agents and scripts: URLs to the
default browser, files to their default (or a named) application, apps by
name, and Finder reveal. One binary, two frontends — a CLI and an MCP stdio
server — over the same exec of /usr/bin/open.

## how it runs

There is no serve process and no store. `{{.Bin}} mcp` is a stdio process
the MCP host spawns; every tool call execs /usr/bin/open with the right
flags, exactly as the CLI commands do. Nothing has to stay running, and
nothing persists between calls. The binary is named opener, not open, so it
can never shadow /usr/bin/open on a PATH that puts personal bins first.

## failure modes

"path is not absolute": deliberate. The MCP server process runs from /, so
a relative path never means what the caller intended — pass a full path or
a ~-prefixed one (the ~ is expanded before the stat).

"no such file or directory" on a path that exists in the conversation: the
file check runs before open does, against the resolved path. The usual
cause is a path from another machine, a sandbox, or a worktree that moved.
Stat the printed path to confirm.

"Unable to find application named ...": open -a takes the name Launch
Services knows, usually the app bundle's display name ("Visual Studio
Code", not "code" or "vscode"). `ls /Applications` shows the names.

A URL is rejected for having no scheme: open sends scheme-less input to
Launch Services as a search or a file, which is rarely what was meant.
Prefix https:// for web pages; every other scheme (mailto:, facetime:,
vscode:) passes through to its registered handler.

The open call succeeds but nothing visibly happens: open returns success
once Launch Services accepts the request. A browser that opened the tab in
an unfocused window, or an app that launched without a window, both look
like silence. There is no signal back from the GUI to check.

MCP tools fail while the CLI works: the MCP host runs whatever binary was
registered, separately from the one on PATH. On macOS an adhoc-signed
binary whose provenance drifted (a manual copy over the installed file) is
killed on exec, which surfaces as the MCP server dying at startup.
Reinstall from the thismoon checkout with `make install` in tools/opener
(it strips xattrs and re-signs), then restart the MCP host.

## version skew

There is no serve process, so skew is between binaries: the MCP host keeps
running the `{{.Bin}} mcp` process it spawned, while `make install`
replaces the binary on PATH. `{{.Bin}} version -o json` reports the
installed build; restart the MCP host (or the session) to pick it up.

## first moves

1. `{{.Bin}} reveal ~` (confirms the binary runs and can drive Finder)
2. `{{.Bin}} url https://example.com` (confirms browser handoff)
3. `{{.Bin}} version -o json` after an install, then restart the MCP host
