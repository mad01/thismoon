# bionic architecture

## Overview

bionic is a tool: a single Go program installed to `~/code/bin/bionic` that
transforms markdown text into bionic reading format (the first half of each
word wrapped in `**bold**`). At runtime it is either a short-lived CLI filter
(text in, text out, exit) or a long-lived MCP stdio server launched by an MCP
host such as Claude Code. Its boundary is stdin/stdout plus the file argument
`render` accepts; it opens no sockets, keeps no state, and never phones home.

## Structure

```
cmd/bionic/          entrypoint; main() delegates to internal/cli.Execute
internal/cli/        cobra command tree: root.go (Version via ldflags),
                     render.go, mcp.go
internal/transform/  the core transform: Bionic(text string) string
internal/mcpserver/  MCP wiring: server.go (New, server name "bionic"),
                     tools.go (registers bionic_render)
```

`internal/transform` is the only Go package with logic; the other three are
frontends and wiring. It is part of the monorepo module
(`github.com/mad01/thismoon/tools/bionic`), with no go.mod of its own.

## Data flow

CLI path: `bionic render [file]` reads the file argument or stdin
(`internal/cli/render.go`), passes the whole text to `transform.Bionic`, and
prints the result to stdout.

MCP path: `bionic mcp` starts the stdio server built by `mcpserver.New`,
which registers the `bionic_render` tool. The tool handler unwraps
`{text: string}` and calls the same `transform.Bionic`, so a shell pipe and
an agent tool call always agree on output.

Inside the transform, `Bionic` splits input on newlines and skips fenced
code blocks entirely. Each remaining line goes through `bionicLine`, which
walks the line rune by rune and calls `boldWord` per word — the first
`(n+1)/2` runes get the bold span. Inline code, existing bold/italic markup,
link URL portions, header prefixes, numbers, and punctuation pass through
unchanged. Table-driven tests in `internal/transform/bionic_test.go` pin
the skip rules.

## Storage

Nothing is stored on disk.

## Interfaces

CLI: `bionic render [file]` (stdin when the argument is omitted or `-`) and
`bionic mcp` (blocks; an MCP host launches it, don't run it by hand).
The version string is injected at build time via ldflags into
`internal/cli.Version`.

MCP: one tool, `bionic_render(text: string) → {text: string}`, registered in
`internal/mcpserver/tools.go`. The server reports its name as `bionic`.

There is no configuration surface: the bold ratio is fixed and no config
file or environment variable is read. Delivery-wise, this component owns
build and install (including the macOS codesign step in the Makefile, needed
for the tool to keep launching as a stdio MCP server); MCP registration is
machine-private wiring in the consuming repo per docs/adr/0006.
