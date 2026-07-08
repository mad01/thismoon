# bionic, CLI and MCP server for bionic reading (half-bold speed-reading format)

Go CLI + MCP server. Transforms text into bionic reading format (first half of each word bold) to improve reading speed. Exposes the same transformation as both a command-line tool and an MCP server for use inside Claude Code.

## Module layout

```
bionic/
  cmd/bionic/
    main.go              entrypoint, delegates to internal/cli.Execute
  internal/
    cli/                 cobra command tree (root, render, mcp); version injected via ldflags
      root.go              root command; Version var set via -ldflags at build time
      render.go            `render` subcommand: reads stdin or file, calls transform.Bionic, prints
      mcp.go                `mcp` subcommand: starts the MCP stdio server
    transform/           core bionic rendering logic
      bionic.go            Bionic(text string) string
      bionic_test.go        table-driven tests: code blocks, bold/italic preservation, numbers, links
    mcpserver/            MCP server wiring
      server.go             builds the MCP server (name "bionic"), registers tools
      tools.go               registers the bionic_render tool
  Makefile              package path github.com/mad01/thismoon/tools/bionic (monorepo module, no own go.mod)
```

## How it works

`cmd/bionic` delegates to `internal/cli.Execute()`, a cobra command tree with two subcommands, `render` and `mcp`. Both ultimately call the same `transform.Bionic(text string) string`, so the CLI and the MCP server produce identical output for the same input.

- `render` reads stdin or a file argument and prints the transformed text.
- `mcp` starts an MCP stdio server (`internal/mcpserver`) that registers the `bionic_render` tool; the tool handler also calls `transform.Bionic`.

### Render algorithm

- `boldWord` takes the first `(n+1)/2` runes of a word and wraps them in a markdown bold span.
- `bionicLine` walks a line rune by rune, calling `boldWord` per word. It skips inline code spans (backtick-delimited), text already wrapped in bold or italic markup, link URL portions `](...)`, and header prefixes `# ...`. Numbers and punctuation pass through unchanged.
- `Bionic` splits input on newlines and skips fenced code blocks entirely before calling `bionicLine` on each remaining line.

Add a test case in `bionic_test.go` before changing the transform logic.

### Change-and-see loop

```sh
# 1. Edit internal/transform/bionic.go
# 2. Run tests
make test

# 3. Try it live
echo "Hello world" | go run ./cmd/bionic render

# 4. Install and verify the MCP server picks it up
make install
claude mcp list
```

## Build / install / test

```bash
make build    # produces ./bionic binary
make install  # build + cp to ~/code/bin/bionic + adhoc codesign
make test     # go test ./...
make tidy     # go mod tidy
make resign   # re-apply adhoc signature without rebuilding (macOS only)
```

Binary installs to `~/code/bin/bionic`.

## Commands

| Command | Description |
|---|---|
| `bionic render [file]` | Convert text to bionic reading format. Reads stdin when you omit a file argument; pass `-` to force stdin. |
| `bionic mcp` | Start the MCP stdio server (blocks; an MCP client such as Claude Code launches this). |

## MCP tools

- `bionic_render(text: string)` → `{ text: string }`
  - Renders markdown text in bionic reading format: the first ~50% of each word is bolded with markdown `**bold**` syntax.
  - Preserves markdown structure. Headers, code blocks, inline code, links, and existing bold/italic markers pass through unchanged; numbers are left untouched.
  - Registered in `internal/mcpserver/tools.go`; the MCP server reports its name as `bionic` (`internal/mcpserver/server.go`).

## Gotchas

- **Wave 0 builder.** Must build before the consuming repo's MCP registration recipe (wave 1) registers it, or `claude mcp` will fail to launch the server.
- **Codesign required for MCP.** macOS 15+ `taskgated` kills adhoc-signed binaries whose provenance xattr no longer matches. `make install` strips xattrs and re-signs. If you manually copy the binary, run `make resign BIN=~/code/bin/bionic`.
- **MCP subcommand blocks.** `bionic mcp` starts the stdio server and doesn't return. An MCP client such as Claude Code launches it; don't run it interactively.

## See also

- Recipe: `recipes/bionic/recipe.toml` (this repo) — build/install, wave 0 (builds before the consuming repo's MCP registration recipe runs)
- MCP registration: the consuming repo's companion recipe registers bionic as transport `stdio` via a sandbox wrapper; the wrapper and its seatbelt profile are machine-private wiring and stay in the consuming repo (two-layer split, see docs/adr/0006)
