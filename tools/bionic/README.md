# bionic

CLI and MCP server that converts text to bionic reading format — the first half of each word is bolded so your eye anchors faster. Use it as a command-line filter (pipe text in, get bolded text out) or as an MCP tool inside Claude Code.

## How it works

Bionic reading bolds the first ~50% of each word using markdown `**bold**` syntax. The idea is that your brain completes familiar words from the bolded anchor, letting you scan faster. The transformer preserves markdown structure: fenced code blocks, inline code, existing bold/italic markers, link URLs, and numbers pass through unchanged.

## Install

```sh
make build && make install
```

This builds `./bionic` and copies it to `~/code/bin/bionic`. On macOS the install also strips quarantine attributes and applies an ad-hoc codesign so macOS Gatekeeper won't block the binary when it runs as an MCP server.

To re-sign an existing binary without rebuilding:

```sh
make resign BIN=~/code/bin/bionic
```

**Requirements:** Go 1.26 or later (the monorepo's `go.mod` pins `go 1.26.2`).

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

Command surface:

| Command | Description |
|---|---|
| `bionic render [file]` | Convert text to bionic reading format. Reads stdin when you omit a file argument. Pass `-` to force stdin. |
| `bionic mcp` | Start the MCP stdio server (blocks; an MCP client such as Claude Code launches this). |

## MCP

`bionic mcp` starts an MCP stdio server that exposes one tool:

| Tool | Input | Description |
|---|---|---|
| `bionic_render` | `text` (string) | Render markdown text in bionic reading format. |

**Automatic registration**: the consuming repo's companion recipe holds MCP registration (machine-private wiring, see docs/adr/0006), and `ralph up` applies it.

**Manual registration** (one-time, user scope):

```sh
claude mcp add --scope user bionic -- bionic mcp
```

Verify:

```sh
claude mcp list
```

The recipe is wave 0, so the binary exists before the MCP registration recipe (wave 1) runs. If you see `claude mcp` fail to launch the server, re-run `make install` and check `claude mcp list`.

## Develop

```sh
make test
# or: go test ./...
```

See `CLAUDE.md` for the module layout, the render algorithm, and the change-and-see loop for editing `internal/transform`.
