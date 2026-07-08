# worklog, resumable cross-session work state (CLI + MCP)

Go CLI + MCP server. Keeps the state of a long, cross-repo task in a stable,
searchable place so a later session can pick it up, keyed by **ticket id or
topic**, never by working directory. Claude Code auto-memory keys on the
working directory: start in `/tmp/foo`, work across three repos, stop, and the
memory is orphaned under a tmp slug you can never find again. worklog keys on
the *task* instead and groups per-repo context underneath it, so that failure
mode can't happen.

## Module layout

```
worklog/
  cmd/worklog/       entrypoint
  internal/
    cli/             cobra command tree (incl. the `mcp` subcommand)
    store/           the on-disk tree: items, per-repo notes, git, repo detection
    mcpserver/       MCP tool wiring (server.go, tools.go)
    config/          optional ~/.config/worklog/config.yaml (ticket-firewall strings)
  Makefile           package path github.com/mad01/thismoon/tools/worklog (monorepo module, no own go.mod)
```

## How it works

The CLI and the MCP server (`worklog mcp`) are two thin frontends over
`internal/store`; both call the same `Checkpoint`, `List`, `Search`, `Load`,
and `SetStatus` methods, so behavior never diverges between an agent call and
a manual `worklog` invocation.

`checkpoint` creates the item if missing, auto-detects the current repo from
cwd (the CLI via `os.Getwd()`, the MCP tool via an explicit `cwd` argument;
see Gotchas), restamps `updated`, rewrites the "Where I am" section, prepends
a "Log" entry, appends a per-repo note, and git-commits the change.

### `scan` and the ticket firewall

`scan` (package `internal/scan`) reads local Claude session transcripts and
emits one compact JSON digest per recent session (repos, tickets, title, first/
last prompts) so `/worklog-backfill` can cluster and import past work without
pulling raw transcripts into context. Window via `--since Nd` or a Go duration;
override the transcript root with `$CLAUDE_PROJECTS_DIR` (used by tests).

`scan` extracts ticket ids from prompt text: uppercase project keys
(`[A-Z]{2,}-\d+`, covering both Linear `MAD-123` and Jira `ABC-1234`), github
issue/PR URLs (`github.com/<o>/<r>/issues|pull/<n>` → `#n`), and bare `#NN`
(addresses issue #55). It also tags each session's `context` (`personal`,
`internal`, `mixed`, `unknown`) from cwd and **filters `tickets[]` to that
world** by key prefix so personal and internal refs never co-mingle: a
personal item carries Linear `MAD-NN` (and legacy `#NN`) but never a Jira key;
an internal item carries Jira keys but never a Linear/github personal ref. The
firewall mirrors the global internal/external separation.

The firewall strings are configurable via `~/.config/worklog/config.yaml`
(override the path with `$WORKLOG_CONFIG`); the built-in defaults cover this
machine layout's generic markers only:

```yaml
scan:
  linear_prefixes: [MAD]                      # TEAM-NN prefixes routed to the personal (Linear) world
  personal_path_markers: ["github.com/mad01/"]
  internal_path_markers: ["/workspace/"]
```

GOPATH-style checkouts of non-github.com hosts count as internal regardless of
config; that split is derived, never enumerated (same principle as belt's
public/internal remote check). The machine-private values ship via the
consuming repo's config overlay (docs/adr/0006).

## Data model & storage

One directory per work item under `~/code/worklog/` (override with
`$WORKLOG_DIR`). The store is a **local git repo with no remote**,
machine-local by design so internal references never leave the machine. csl
indexes it for free.

```
~/code/worklog/<key>/
  CONTEXT.md          # frontmatter (status, ticket, topic, repos[], created, updated, last_cwd)
                      # body: "Where I am" (rewritten each checkpoint) + "Log" (append-only, reverse-chron)
  repos/<repo>.md     # per-repo notes, created lazily; checkpoint auto-detects the repo from cwd
  artifacts/          # optional: diffs, plans, links
```

`<key>` is a ticket id (`ABC-1234`) passed through unchanged, or a slugified
topic. The CONTEXT.md contract is borrowed from an earlier internal experiment.
Deliberately skipped: stage folders, per-stage state.json,
context-pipeline.json, agent-card.json.

## Build / install / test

```bash
make build    # ./worklog
make install  # build + cp to ~/code/bin/worklog + adhoc codesign
make test     # go test ./...
```

## Commands

```
worklog checkpoint <key> --where "..." --note "..." [--repo R] [--ticket ID]
worklog list [--status active|paused|done] [--repo R]
worklog show <key> [--repo R]
worklog search <query>
worklog status <key> active|paused|done
worklog new <key> [--ticket ID]
worklog path [key]
worklog scan --since 14d   # digest ~/.claude/projects/*/*.jsonl as JSON; see How it works
worklog mcp        # MCP stdio server (blocks)
```

## MCP tools

`worklog mcp` starts the MCP stdio server and registers five `worklog_*`
tools (`internal/mcpserver/tools.go`). Handlers call the same store methods as
the CLI (see How it works). `worklog_checkpoint`, `worklog_list`,
`worklog_search`, and `worklog_status` return an item view:
`{key, status, ticket?, topic?, updated, repos?}`.

- `worklog_checkpoint(key, where?, note?, ticket?, topic?, repo?, cwd?)` → item
  view. Requires `where` and/or `note`. Creates the item if missing; `ticket`/
  `topic` only apply on first creation. `cwd` is the user's working
  directory: pass it so the tool can detect the repo, since the MCP server
  process itself runs from `/` (see Gotchas); `repo` overrides detection.
  `where` is the ONLY context a fresh session gets when resuming. Write it
  for a reader with zero prior knowledge: goal, repo paths, conclusions
  reached and why, working tool/query examples with exact parameters,
  anti-patterns that waste time, links to tickets/docs/PRs, decisions made and
  their reasoning, and concrete next steps.
- `worklog_list(status?, repo?)` → `{items: itemView[]}`. Filter by status
  (`active`, `paused`, or `done`) or repo, newest first.
- `worklog_search(query)` → `{items: itemView[]}`. Substring match against key
  and content (CONTEXT.md and repo notes), active items first, then recency.
- `worklog_show(key, repo?)` → `{markdown}`. The item's full CONTEXT.md, or a
  per-repo note's markdown when `repo` is set.
- `worklog_status(key, status)` → item view. `status` must be `active`,
  `paused`, or `done`.

## Gotchas

- **Wave 0 builder.** Must build before the consuming repo's MCP registration
  recipe (wave 1) registers `worklog mcp`.
- **Codesign required for MCP.** `make install` strips xattrs and re-signs.
  Manual copy → `make resign BIN=~/code/bin/worklog`.
- **Repo auto-detect needs a real cwd.** The MCP server process runs from `/`,
  so the `worklog_checkpoint` tool takes a `cwd` argument; the skill passes
  the user's working directory. The CLI uses `os.Getwd()` directly.
- **No remote.** Cross-machine sync is intentionally out of scope for now.
  Resume on the machine you left.

## See also

- Recipe: `recipes/worklog/recipe.toml` (this repo: build/install; wave 0,
  builds before MCP registration)
- MCP registration (consuming repo): `worklog mcp` registered with the MCP
  host, unsandboxed as first-party code
- Config overlay (consuming repo): the companion recipe ships
  `~/.config/worklog/config.yaml` with the machine's firewall strings
- Permissions + skills (consuming repo): `Bash(worklog:*)` /
  `mcp__worklog__*` permissions and the worklog + worklog-backfill Claude
  skills reference personal paths and ticket-key conventions, so they stay
  machine-private
- See docs/adr/0006 for the two-layer build/install vs. machine-private
  wiring split
