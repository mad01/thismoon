# opener architecture

## Overview

opener is a tool: one Go program installed to `~/code/bin/opener` bridging
the macOS open command. At runtime it is a short-lived CLI or an MCP stdio
server (`opener mcp`); nothing stays running between invocations, and there
is no store — every operation is one exec of /usr/bin/open, and Launch
Services owns everything after that.

## Structure

```
cmd/opener/          entrypoint
internal/cli/        cobra command tree, including the mcp subcommand
internal/sysopen/    open core: Opener.URL / File / App / With / Reveal
                     over exec of /usr/bin/open, plus path and URL
                     validation (resolvePath, scheme check)
internal/mcpserver/  MCP wiring (server.go, tools.go)
```

`internal/sysopen` owns the exec boundary and all validation.
`internal/cli` and `internal/mcpserver` are two thin frontends calling the
same Opener methods, so an agent tool call and a manual CLI run always
behave identically.

## Data flow

Every verb validates first, then execs: URL parses the input and requires a
scheme; File, With, and Reveal expand a leading ~, require the result
absolute (the MCP server process runs from /), and stat it; App requires a
non-empty name. Validation failures never exec. The exec maps verbs to
flags — URL/File pass the target bare, App is `-a <name>`, With is
`-a <app> <path>`, Reveal is `-R <path>` — with stderr captured onto the
error, so "Unable to find application" surfaces verbatim. Errors cross the
boundary wrapped by `agentdoc.Hint`, pointing at `opener docs`.

## Interfaces

CLI: `url`, `file`, `app`, `with`, `reveal`, `docs`, `mcp`, and `version`
(bare token, or the shared four-key build metadata object with `-o json`)
via the shared `github.com/mad01/thismoon/buildinfo` package. Every open
command prints `opened <target>` with the resolved target.

MCP: `opener mcp` starts a stdio server registering five tools —
`open_url(url)`, `open_file(path)`, `open_app(name)`,
`open_with(path, app)`, `reveal_in_finder(path)` — each returning
`{opened}` with the URL, resolved path, or app name it handed to open. The
runner inside `sysopen.Opener` is a swappable function field — the test
seam that keeps tests from launching apps and browser tabs.
