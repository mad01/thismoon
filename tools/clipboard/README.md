# clipboard

macOS system clipboard bridge: a CLI and MCP server over pbcopy/pbpaste.

Copying text out of an agent session used to mean piping through the shell:
a permission prompt and a quoting hazard for something as small as "put this
command on my clipboard." clipboard makes the pasteboard a first-class tool:
structured copy and paste with no shell in the middle.

## Install

```bash
make install   # builds and installs to ~/code/bin/clipboard
```

## Usage

```bash
clipboard copy "kubectl get pods -n prod"   # argument form
git log -1 --format=%H | clipboard copy     # stdin form
clipboard paste                             # print the clipboard verbatim
```

Text moves verbatim in both directions: clipboard never trims it and never
appends a newline.

## MCP

`clipboard mcp` starts the MCP stdio server, exposing the same operations as
tools so an agent can use the clipboard without shelling out:
`clipboard_copy(text)` and `clipboard_paste()`.

On a standalone install, register it once:

```sh
claude mcp add --scope user clipboard -- clipboard mcp
```

On a ralph-managed machine, skip the manual command: registration is
machine-private wiring that ships from the consuming repo's companion
recipe (`docs/adr/0006` at the repo root). See [`CLAUDE.md`](CLAUDE.md)
for the two-layer build/install vs. wiring split.

No backing service has to be running: the tools shell out to
`pbcopy`/`pbpaste` in-process, macOS only. Confirm the server is registered
with `claude mcp list`, which should list `clipboard` among the connected
servers. There is no `clipboard doctor`; `clipboard docs` prints the
embedded operating doc (pasteboard failure modes, the empty-paste case)
instead.

## Develop

```bash
make build    # ./clipboard
make test     # go test ./...
```

See [`CLAUDE.md`](CLAUDE.md) for the module layout and MCP tool signatures.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
