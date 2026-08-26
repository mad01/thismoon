# opener

macOS open bridge: a CLI and MCP server over the system `open` command.

Opening things out of an agent session used to mean a shell call — a
permission prompt and hand-quoted paths for something as small as "open
this link." opener makes the open command a first-class tool: URLs, files,
apps, and Finder reveal as structured operations that validate before they
launch.

The binary is named opener so it can never shadow `/usr/bin/open` on a
PATH that puts personal bins first.

## Install

```bash
make install   # builds and installs to ~/code/bin/opener
```

## Usage

```bash
opener url https://github.com/mad01/thismoon   # default browser
opener file ~/Downloads/report.pdf             # default app for the file
opener app Safari                              # launch or foreground
opener with ~/notes.md "Visual Studio Code"    # a specific app
opener reveal ~/code/bin                       # show in Finder, selected
```

Paths must be absolute or ~-prefixed and must exist; URLs must carry a
scheme. Validation runs before anything launches, so a typo fails with a
clear error instead of a GUI dialog.

## MCP

`opener mcp` starts the MCP stdio server, exposing the same operations as
tools so an agent can open things without shelling out: `open_url`,
`open_file`, `open_app`, `open_with`, `reveal_in_finder`.

Registration is machine-private: the consuming repo's companion recipe
registers `opener mcp` with the MCP host. See [`CLAUDE.md`](CLAUDE.md) for
the two-layer build/install vs. wiring split.

## Develop

```bash
make build    # ./opener
make test     # go test ./...
```

See [`CLAUDE.md`](CLAUDE.md) for the module layout and MCP tool signatures.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
