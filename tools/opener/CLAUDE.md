# opener, macOS open bridge (CLI + MCP)

Go CLI + MCP server wrapping the macOS `open` command: URLs to the default
browser, files to their default (or a named) application, apps by name, and
Finder reveal. Exists so an agent can act on "open this" without a shell
call, a permission prompt, or quoting bugs.

Named **opener**, not open: a binary named open would shadow /usr/bin/open
on any PATH that puts personal bins first. The MCP tool names keep the
`open_` prefix.

## Module layout

```
opener/
  cmd/opener/        entrypoint
  internal/
    cli/             cobra command tree (incl. the `mcp` subcommand)
    sysopen/         open core: URL/File/App/With/Reveal over exec + validation
    mcpserver/       MCP tool wiring (server.go, tools.go)
  Makefile           package path github.com/mad01/thismoon/tools/opener (monorepo module, no own go.mod)
```

## How it works

The CLI and the MCP server (`opener mcp`) are two thin frontends over
`internal/sysopen`; both call the same `Opener` methods, so behavior never
diverges between an agent call and a manual invocation. Every verb
validates before it execs: URLs must carry a scheme; paths get ~ expanded,
must be absolute (the MCP server process runs from `/`), and must exist.
The exec targets /usr/bin/open by absolute path, since MCP hosts spawn
processes with a minimal PATH; stderr rides the returned error, so Launch
Services failures ("Unable to find application named ...") surface
verbatim.

## Build / install / test

```bash
make build    # ./opener (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/opener + adhoc codesign
make test     # go test ./...
```

## Commands

```
opener url <url>           # default handler for the scheme (browser for https)
opener file <path>         # default application for the file
opener app <name>          # launch/foreground by name (open -a)
opener with <path> <app>   # open the file with a specific app (open -a app path)
opener reveal <path>       # show in Finder, selected (open -R)
opener docs                # print the embedded operating doc
opener mcp                 # MCP stdio server (blocks)
opener version [-o json]
```

## MCP tools

All five return `{opened}` — the URL, resolved path, or app name handed to
the open command — for confirmation.

- `open_url(url)`: requires a scheme; non-http schemes go to their handler.
- `open_file(path)`: absolute or ~-prefixed, must exist.
- `open_app(name)`: the name Launch Services knows ("Visual Studio Code").
- `open_with(path, app)`: file with a specific app instead of its default.
- `reveal_in_finder(path)`: Finder window with the path selected.

## Gotchas

- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `opener docs`): path and app-name failure modes, the
  silent-success case, codesign-for-MCP. Keep those facts there, not here.
- **Tests never launch anything.** `sysopen.Opener`'s runner is a swappable
  function field; tests inject a fake. Don't add a test that execs open —
  it would pop windows on whoever runs the suite.
- **Validation precedes exec.** A rejected URL or path returns before
  /usr/bin/open runs; keep it that way, the pre-exec stat is what turns
  open's GUI-flavored errors into parseable ones.
- **Wave 0 builder.** Must build before the consuming repo's MCP
  registration recipe (wave 1) registers `opener mcp`.
- **Version probe convention.** `opener version -o json` returns the shared
  four-key build metadata object (`version`, `commit`, `tag`, `build_time`)
  from `github.com/mad01/thismoon/buildinfo`; plain `opener version` stays
  a bare token that status parses. opener is CLI + MCP only, so there is no
  `/version` endpoint.

## See also

- Recipe: `recipes/opener/recipe.toml` (this repo: build/install; wave 0,
  builds before MCP registration)
- MCP registration (consuming repo): `opener mcp` registered with the MCP
  host — machine-private wiring per docs/adr/0006
