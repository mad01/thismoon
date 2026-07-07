# bionic

CLI and MCP server that converts text to bionic reading format — the first half of each word is bolded so your eye anchors faster.

Bionic reading bolds the first ~50% of each word using markdown `**bold**` syntax. The idea is that your brain completes familiar words from the bolded anchor, letting you scan faster. `bionic` exposes this transformation as both a command-line tool (pipe text in, get bolded text out) and an MCP server for use inside Claude Code.

The transformer preserves markdown structure: fenced code blocks, inline code, existing bold/italic markers, link URLs, and numbers pass through unchanged.

## Install

```sh
make build && make install
```

This builds `./bionic` and copies it to `~/code/bin/bionic`. On macOS the install also strips quarantine attributes and applies an ad-hoc codesign so macOS Gatekeeper won't block the binary when it runs as an MCP server.

To re-sign an existing binary without rebuilding:

```sh
make resign BIN=~/code/bin/bionic
```

**Requirements:** Go 1.23 or later.

## Usage

Read from stdin:

```sh
echo "The quick brown fox jumps over the lazy dog" | bionic render
```

Output:

```
**Th**e **qui**ck **bro**wn **fo**x **jum**ps **ov**er **th**e **la**zy **do**g
```

Read from a file:

```sh
bionic render README.md
```

Pipe a markdown file through a pager to read it in bionic format:

```sh
bionic render notes.md | glow -
```

## Commands

| Command | Description |
|---|---|
| `bionic render [file]` | Convert text to bionic reading format. Reads stdin when you omit a file argument. Pass `-` to force stdin. |
| `bionic mcp` | Start the MCP stdio server (blocks; an MCP client such as Claude Code launches this). |

## MCP server

`bionic mcp` starts an MCP stdio server that exposes one tool:

| Tool | Input | Description |
|---|---|---|
| `bionic_render` | `text` (string) | Render markdown text in bionic reading format. |

**Automatic registration**: MCP registration lives in the consuming repo's companion recipe (machine-private wiring, see docs/adr/0006) and is applied by `ralph up`.

**Manual registration** (one-time, user scope):

```sh
claude mcp add --scope user bionic -- bionic mcp
```

Verify:

```sh
claude mcp list
```

The recipe is wave 0, so the binary exists before the MCP registration recipe (wave 1) runs. If you see `claude mcp` fail to launch the server, re-run `make install` and check `claude mcp list`.

## Working on it

**Key files:**

| File | What it does |
|---|---|
| `internal/transform/bionic.go` | Core transform: `Bionic(text string) string`. Splits on newlines, skips fenced code blocks, calls `bionicLine` per line. |
| `internal/transform/bionic_test.go` | Table-driven tests covering code blocks, bold/italic preservation, numbers, links. |
| `internal/cli/render.go` | `render` subcommand: reads stdin or file, calls `transform.Bionic`, prints. |
| `internal/mcpserver/tools.go` | MCP tool registration for `bionic_render`. |
| `Makefile` | `build`, `install`, `resign`, `test`, `tidy`, `clean`. |

**Change-and-see loop:**

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

**Render logic:** `boldWord` takes the first `(n+1)/2` runes of a word and wraps them in `**...**`. `bionicLine` walks runes and skips: inline code spans (backtick-delimited), existing `**`/`__` bold markers, italic markers, link URL portions `](...)`, and header prefixes `# ...`. Numbers and punctuation fall through unchanged. Add a test case in `bionic_test.go` before changing the transform logic.

**Tests:**

```sh
make test
# or: go test ./...
```
