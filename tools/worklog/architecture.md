# worklog architecture

## Overview

worklog is a tool: one Go program installed to `~/code/bin/worklog` that
keeps resumable work state for long, cross-repo tasks, keyed by ticket id or
topic. At runtime it is a short-lived CLI or an MCP stdio server
(`worklog mcp`); nothing stays running between invocations. All state is a
markdown tree under a local git repo; every operation is a read or write of
that tree, and the tool's boundary is that directory plus (for `scan`) a
read-only pass over local Claude Code session transcripts.

## Structure

```
cmd/worklog/         entrypoint
internal/cli/        cobra command tree, including the mcp subcommand
internal/store/      the on-disk tree: store.go (root, git plumbing, List,
                     Search), item.go (Checkpoint, Load, SetStatus,
                     RepoNote), repo.go (DetectRepo from cwd)
internal/scan/       session-transcript digestion and the ticket firewall
internal/config/     optional ~/.config/worklog/config.yaml (firewall
                     strings), path override via $WORKLOG_CONFIG
internal/mcpserver/  MCP wiring (server.go, tools.go)
```

`internal/store` owns all state transitions. `internal/cli` and
`internal/mcpserver` are two thin frontends calling the same store methods,
so an agent tool call and a manual CLI run always see identical behavior.

## Data flow

Checkpoint: `worklog checkpoint <key>` (or `worklog_checkpoint`) reaches
`store.Checkpoint`. It loads the item or creates it, detects the current
repo via `DetectRepo` (the CLI feeds it `os.Getwd()`; the MCP handler passes
an explicit `cwd` argument, since the server process runs from `/`), then
restamps `updated`, rewrites the "Where I am" section (`replaceSection`),
prepends a "Log" entry (`prependLog`), appends a per-repo note
(`appendRepoNote`), writes the item, and git-commits the store.

Reads: `list` and `search` walk the item directories (`store.List`,
`store.Search`; search is a substring match over CONTEXT.md and repo notes,
active items first). `show` returns an item's CONTEXT.md or one repo note;
`status` rewrites the frontmatter status and commits.

Scan: `worklog scan --since 14d` runs `scan.Scan` over
`~/.claude/projects/*/*.jsonl` (root override: `$CLAUDE_PROJECTS_DIR`).
`digestFile` reduces each transcript to a compact session digest (repos,
tickets, first/last prompts); `classifyCwd` tags the session personal or
internal from its paths, and `resolveTickets` filters extracted ticket keys
to that world — the ticket firewall. Output is JSON on stdout; scan never
writes to the store.

## Storage

The store root is `~/code/worklog/` (override: `$WORKLOG_DIR`), a local git
repo with no remote, initialized on first write. One directory per item:

```
<key>/
  CONTEXT.md       frontmatter (status, ticket, topic, repos, created,
                   updated, last_cwd); body: "Where I am" + reverse-chron Log
  repos/<repo>.md  per-repo notes, created lazily
  artifacts/       optional extras (diffs, plans, links)
```

Every mutation is a git commit, so history is queryable with plain git. The
optional config file at `~/.config/worklog/config.yaml` is read, never
written.

## Interfaces

CLI: `checkpoint`, `list`, `show`, `search`, `status`, `new`, `path`,
`scan`, `mcp`, and `version` (bare token, or the shared four-key build
metadata object with `-o json`). The build metadata is injected via ldflags
into the shared `github.com/mad01/thismoon/buildinfo` package, which also
backs the `worklog --version` flag and the MCP server's reported version.

MCP: `worklog mcp` starts a stdio server registering five tools
(`worklog_checkpoint`, `worklog_list`, `worklog_search`, `worklog_show`,
`worklog_status`); the handlers call the same store methods as the CLI.
`worklog_checkpoint` differs from its CLI twin only in taking `cwd` as an
argument for repo detection.

Config surfaces: `$WORKLOG_DIR` (store root), `$WORKLOG_CONFIG` and
`~/.config/worklog/config.yaml` (scan firewall strings: linear prefixes,
path markers, checkout roots), `$CLAUDE_PROJECTS_DIR` (transcript root, used
by tests). The machine-private firewall values ship as a config overlay from
the consuming repo per docs/adr/0006; the built-in defaults carry only
generic markers.
