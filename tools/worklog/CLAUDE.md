# worklog — resumable cross-session work state (CLI + MCP)

Go CLI + MCP server. Keeps the state of a long, cross-repo task in a stable,
searchable place so a later session can pick it up — keyed by **ticket id or
topic**, never by working directory. Solves the failure mode where work started
in a tmp dir lands in a dir-slug memory you can never find again.

## Why it exists

Claude Code auto-memory keys on the working directory. Start in `/tmp/foo`, work
across three repos, stop — the memory is orphaned under a tmp slug. worklog keys
on the *task* instead, and groups per-repo context underneath it.

## Store layout

One directory per work item under `~/code/worklog/` (override with `$WORKLOG_DIR`).
The store is a **local git repo with no remote** — machine-local by design so
internal references never leave the machine. csl indexes it for free.

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

## Module layout

```
worklog/
  cmd/worklog/       — entrypoint
  internal/
    cli/             — cobra command tree (incl. the `mcp` subcommand)
    store/           — the on-disk tree: items, per-repo notes, git, repo detection
    mcpserver/       — MCP tool wiring (server.go, tools.go)
    config/          — optional ~/.config/worklog/config.yaml (ticket-firewall strings)
  Makefile           — package path github.com/mad01/thismoon/tools/worklog (monorepo module, no own go.mod)
```

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
worklog scan --since 14d   # digest ~/.claude/projects/*/*.jsonl as JSON (drives /worklog-backfill)
worklog mcp        # MCP stdio server (blocks)
```

`scan` (package `internal/scan`) reads local Claude session transcripts and
emits one compact JSON digest per recent session (repos, tickets, title, first/
last prompts) so `/worklog-backfill` can cluster and import past work without
pulling raw transcripts into context. Window via `--since Nd` or a Go duration;
override the transcript root with `$CLAUDE_PROJECTS_DIR` (used by tests).

`scan` extracts ticket ids from prompt text — uppercase project keys
(`[A-Z]{2,}-\d+`, covering both Linear `MAD-123` and Jira `ABC-1234`), github
issue/PR URLs (`github.com/<o>/<r>/issues|pull/<n>` → `#n`), and bare `#NN`
(addresses issue #55). It also tags each session's `context` (`personal`,
`internal`, `mixed`, `unknown`) from cwd and **filters `tickets[]` to that
world** by key prefix so personal and internal refs never co-mingle — a
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
config — that split is derived, never enumerated (same principle as belt's
public/internal remote check). The machine-private values ship via the
consuming repo's config overlay (docs/adr/0006).

`checkpoint` creates the item if missing, auto-detects the current repo from
cwd, restamps `updated`, rewrites "Where I am", prepends a "Log" entry, appends
a per-repo note, and git-commits.

## Wired in via (two-layer split, see docs/adr/0006)

- **Build/install (this repo):** `recipes/worklog/recipe.toml` (wave 0 — builds before MCP registration).
- **MCP registration (consuming repo):** `worklog mcp` registered with the MCP host, unsandboxed — first-party code.
- **Config overlay (consuming repo):** the companion recipe ships `~/.config/worklog/config.yaml` with the machine's firewall strings.
- **Permissions + skills (consuming repo):** `Bash(worklog:*)` / `mcp__worklog__*` permissions and the worklog + worklog-backfill Claude skills reference personal paths and ticket-key conventions, so they stay machine-private.

## Gotchas

- **Wave 0 builder.** Must build before the consuming repo's MCP registration recipe (wave 1) registers `worklog mcp`.
- **Codesign required for MCP.** `make install` strips xattrs and re-signs. Manual copy → `make resign BIN=~/code/bin/worklog`.
- **Repo auto-detect needs a real cwd.** The MCP server process runs from `/`, so the `worklog_checkpoint` tool takes a `cwd` argument; the skill passes the user's working directory. The CLI uses `os.Getwd()` directly.
- **No remote.** Cross-machine sync is intentionally out of scope for now — resume on the machine you left.

## See also

- Recipe: `recipes/worklog/recipe.toml` (this repo — build/install)
- MCP registration, config overlay, and skills: the consuming repo's companion recipe (machine-private wiring, docs/adr/0006)
