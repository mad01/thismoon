# bionic — CLI and MCP server for bionic reading (half-bold speed-reading format)

Go CLI + MCP server. Transforms text into bionic reading format (first half of each word bold) to improve reading speed. Exposes the same transformation as both a command-line tool and an MCP server for use inside Claude Code.

## Module layout

```
bionic/
  cmd/bionic/        — entrypoint (delegates to internal/cli)
  internal/
    cli/             — cobra command tree (root, mcp subcommand); version injected via ldflags
    transform/       — core bionic rendering logic
    mcpserver/       — MCP server wiring (tools.go, server.go)
  Makefile           — package path github.com/mad01/thismoon/tools/bionic (monorepo module, no own go.mod)
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

## Wired in via (two-layer split, see docs/adr/0006)

- **Build/install (this repo):** `recipes/bionic/recipe.toml` (wave 0 — builds before MCP registration)
- **MCP registration (consuming repo):** registered as transport `stdio` via the companion recipe's sandbox wrapper — the wrapper and its seatbelt profile are machine-private wiring and stay in the consuming repo

## Gotchas

- **Wave 0 builder.** Must build before the consuming repo's MCP registration recipe (wave 1) registers it, or `claude mcp` will fail to launch the server.
- **Codesign required for MCP.** macOS 15+ `taskgated` kills adhoc-signed binaries whose provenance xattr no longer matches. `make install` strips xattrs and re-signs. If you manually copy the binary, run `make resign BIN=~/code/bin/bionic`.
- MCP subcommand is `bionic mcp` — the server blocks on stdio.

## See also

- Recipe: `recipes/bionic/recipe.toml` (this repo — build/install)
- MCP registration + sandbox wrapper: the consuming repo's companion recipe (machine-private wiring, docs/adr/0006)
