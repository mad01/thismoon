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
    config/          optional config.yaml (ticket-firewall strings, store remote); path via kit/confdir
    scan/            reads Claude session transcripts, emits per-session JSON digests (see `scan` below)
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

The firewall strings live in worklog's config file (`--config`, else
`$WORKLOG_CONFIG`, else `config.yaml` under `$XDG_CONFIG_HOME`/`~/.config`):

```yaml
scan:
  linear_prefixes: [MAD]                      # TEAM-NN prefixes routed to the personal (Linear) world
  personal_path_markers: ["github.com/you/"]
  internal_path_markers: ["/workspace/"]
  checkout_roots: ["/code/src/"]              # GOPATH-style roots; the next segment is read as the git host
  repo_path_markers: ["/code/", "/workspace/"] # a cwd matching none of these reports no repo
```

The three classification keys ship with **no built-in values** — they
describe one person's machine layout, so a compiled-in guess would misfile
another machine's sessions. Unconfigured, the firewall routes nothing: every
session is `context: "unknown"` with all its ticket refs surfaced for review.
Only the two path-shape keys (`checkout_roots`, `repo_path_markers`) have
defaults. GOPATH-style checkouts of non-github.com hosts count as internal
(host derived from the path segment after a checkout root); that split is
derived, never enumerated (same principle as belt's public/internal remote
check), which is why it can ship with a default when the marker lists cannot.
The machine's values ship via the consuming repo's config overlay
(docs/adr/0006).

## Data model & storage

One directory per work item under `~/code/worklog/` (override with
`$WORKLOG_DIR`). The store is a **git repo, local-first**: with no
`remote.url` in the config it has no upstream at all, and with one it clones
from and pushes to a private per-machine repo. Which machine gets which URL
is decided when the machine is provisioned — the config overlay installs the
right one — so a work machine's store, internal references included, only
ever reaches its own private repo. csl indexes the store for free.

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
make build    # ./worklog (build metadata via ldflags, from ../../buildinfo.mk)
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
worklog sync       # fast-forward pull then push against the configured remote
worklog scan --since 14d   # digest ~/.claude/projects/*/*.jsonl as JSON; see How it works
worklog config     # config file location + the settings in effect; --help carries the annotated reference
worklog docs       # print the embedded operating doc (runtime debugging for agents and humans)
worklog mcp        # MCP stdio server (blocks)
worklog version [-o json]
```

`worklog version` prints the bare git commit that built the binary, the token
sibling tools also print so ralph and status can probe any of them for the
build they are running; `-o json` prints the full build metadata object. The
cobra `worklog --version` flag prints the same token in cobra's own phrasing.

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
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `worklog docs`): codesign-for-MCP, store and push failure modes,
  version-skew checks. Keep those facts there, not here.
- **Repo auto-detect needs a real cwd.** The MCP server process runs from `/`,
  so the `worklog_checkpoint` tool takes a `cwd` argument; the skill passes
  the user's working directory. The CLI uses `os.Getwd()` directly.
- **Remote sync is config-driven and single-writer.** `remote.url` is the
  upstream; worklog auto-configures `origin`, clones it when the store dir is
  missing (fresh machine), and commits+pushes after every write (`push: false`
  turns the push off). `worklog sync` does a fast-forward pull then push.
  There is no merge strategy: the assumption is one writer at a time, so
  concurrent checkpoints of the same item from two machines will conflict —
  run `worklog sync` when switching machines. A failed push degrades to a
  warning; the write always lands locally.
- **`remote.upstreams` is retired.** The profile-keyed upstream map, resolved
  by reading ralph's `config.local.toml`, is gone: a machine's class is
  written once at provisioning time and does not change between runs, so the
  overlay installs the machine's own `url` (same conclusion as ADR-0010 for
  the guard tools). A config still carrying `upstreams` with no `url` gets a
  stderr warning rather than a store that quietly stopped pushing.
- **A broken config is fatal everywhere but `worklog config`.** A missing file
  is fine (defaults); a file that exists and will not read or parse stops the
  command, because it decides where writes get pushed. `worklog config` prints
  the error and the defaults instead — it is the command you run to find out
  what broke.
- **Version probe convention.** `worklog version -o json` returns the shared
  four-key build metadata object (`version`, `commit`, `tag`, `build_time`,
  every key present and `""` when unknown) from
  `github.com/mad01/thismoon/buildinfo`, so ralph can check which build is
  installed. Plain `worklog version` stays a bare token — status parses it as
  one. worklog is CLI + MCP only, so there is no `/version` endpoint; the MCP
  server reports the same token as its server version.

## See also

- Recipe: `recipes/worklog/recipe.toml` (this repo: build/install; wave 0,
  builds before MCP registration)
- MCP registration (consuming repo): `worklog mcp` registered with the MCP
  host, unsandboxed as first-party code
- Config overlay (consuming repo): the companion recipe ships
  `~/.config/worklog/config.yaml` with the machine's firewall strings and its
  own `remote.url` — both are now machine-specific with no compiled fallback
- Permissions + skills (consuming repo): `Bash(worklog:*)` /
  `mcp__worklog__*` permissions and the worklog + worklog-backfill Claude
  skills reference personal paths and ticket-key conventions, so they stay
  machine-private
- See docs/adr/0006 for the two-layer build/install vs. machine-private
  wiring split
