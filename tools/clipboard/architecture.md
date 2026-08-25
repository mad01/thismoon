# clipboard architecture

## Overview

clipboard is a tool: one Go program installed to `~/code/bin/clipboard`
bridging the macOS system pasteboard. At runtime it is a short-lived CLI or
an MCP stdio server (`clipboard mcp`); nothing stays running between
invocations, and there is no store — the pasteboard itself is the only
state.

## Structure

```
cmd/clipboard/       entrypoint
internal/cli/        cobra command tree, including the mcp subcommand
internal/clip/       pasteboard core: Board.Copy / Board.Paste over exec
                     of /usr/bin/pbcopy and /usr/bin/pbpaste
internal/mcpserver/  MCP wiring (server.go, tools.go)
```

`internal/clip` owns the exec boundary. `internal/cli` and
`internal/mcpserver` are two thin frontends calling the same Board methods,
so an agent tool call and a manual CLI run always behave identically.

## Data flow

Copy: `clipboard copy` (or `clipboard_copy`) reaches `clip.Board.Copy`,
which execs pbcopy with the text on stdin, verbatim. Paste:
`clipboard paste` (or `clipboard_paste`) reaches `clip.Board.Paste`, which
execs pbpaste and returns its stdout, verbatim. Errors cross the boundary
wrapped by `agentdoc.Hint`, pointing at `clipboard docs`.

## Interfaces

CLI: `copy [text]` (stdin when no argument), `paste`, `docs`, `mcp`, and
`version` (bare token, or the shared four-key build metadata object with
`-o json`) via the shared `github.com/mad01/thismoon/buildinfo` package.

MCP: `clipboard mcp` starts a stdio server registering two tools:
`clipboard_copy(text)` → `{copied_bytes}` and `clipboard_paste()` →
`{text, note?}`, where `note` appears only on empty text and names why an
empty result is expected. The runner inside `clip.Board` is a swappable
function field — the test seam that keeps tests off the real clipboard.
