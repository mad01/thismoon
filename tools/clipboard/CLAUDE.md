# clipboard, macOS system clipboard bridge (CLI + MCP)

Go CLI + MCP server wrapping the macOS pasteboard commands: `copy` puts text
on the clipboard via pbcopy, `paste` reads it back via pbpaste. Exists so an
agent can hand text to the user (a command, a link, a snippet) without a
shell call, a permission prompt, or quoting bugs.

## Module layout

```
clipboard/
  cmd/clipboard/     entrypoint
  internal/
    cli/             cobra command tree (incl. the `mcp` subcommand)
    clip/            pasteboard core: Board.Copy / Board.Paste over exec
    mcpserver/       MCP tool wiring (server.go, tools.go)
  Makefile           package path github.com/mad01/thismoon/tools/clipboard (monorepo module, no own go.mod)
```

## How it works

The CLI and the MCP server (`clipboard mcp`) are two thin frontends over
`internal/clip`; both call the same `Board.Copy` and `Board.Paste`, so
behavior never diverges between an agent call and a manual invocation. The
Board execs /usr/bin/pbcopy (text on stdin) and /usr/bin/pbpaste (stdout
captured) — absolute paths, since MCP hosts spawn processes with a minimal
PATH. Text is copied and pasted verbatim: no trimming, no newline appended.

## Build / install / test

```bash
make build    # ./clipboard (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/clipboard + adhoc codesign
make test     # go test ./...
```

## Commands

```
clipboard copy [text]    # no argument: read stdin
clipboard paste          # print the clipboard verbatim (no newline appended)
clipboard docs           # print the embedded operating doc
clipboard mcp            # MCP stdio server (blocks)
clipboard version [-o json]
```

## MCP tools

- `clipboard_copy(text)` → `{copied_bytes}`. Replaces the clipboard with
  `text` verbatim; rejects empty text.
- `clipboard_paste()` → `{text, note?}`. `note` is set only when `text` is
  empty and names why (empty clipboard, or non-text content pbpaste cannot
  render).

## Gotchas

- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `clipboard docs`): pasteboard failure modes, the empty-paste
  case, codesign-for-MCP. Keep those facts there, not here.
- **Tests never touch the real clipboard.** `clip.Board`'s runner is a
  swappable function field; tests inject a fake. Don't add a test that execs
  pbcopy — it would clobber whatever the user has copied.
- **Wave 0 builder.** Must build before the consuming repo's MCP
  registration recipe (wave 1) registers `clipboard mcp`.
- **Version probe convention.** `clipboard version -o json` returns the
  shared four-key build metadata object (`version`, `commit`, `tag`,
  `build_time`) from `github.com/mad01/thismoon/buildinfo`; plain
  `clipboard version` stays a bare token that status parses. clipboard is
  CLI + MCP only, so there is no `/version` endpoint.

## See also

- Recipe: `recipes/clipboard/recipe.toml` (this repo: build/install; wave 0,
  builds before MCP registration)
- MCP registration (consuming repo): `clipboard mcp` registered with the MCP
  host — machine-private wiring per docs/adr/0006
