# csl

> Local code search over your git checkouts. A CLI and an MCP stdio server for Claude Code, backed by [zoekt](https://github.com/sourcegraph/zoekt).

`csl` walks the directories you configure, indexes every git repo it finds, and searches them with zoekt's trigram index. You get `grep`-like queries across dozens of checkouts in milliseconds, without shipping your code to a cloud service.

There is no server to manage. A short-lived search daemon is spawned on demand from the same binary, keeps zoekt shards mmap'd across queries, and exits after ten minutes of idleness.

Use it in two ways:

- At the terminal: `csl search`, `csl count`, `csl repo`, `csl read`.
- Inside Claude Code (or any MCP client). `csl mcp` exposes eight `csl_*` tools so the agent can search, resolve repo paths, and read files without shelling out.

## Install

Requires Go 1.25+.

```sh
go install github.com/mad01/thismoon/services/csl/cmd/csl@latest
```

From a checkout:

```sh
make install   # builds with version embedded, copies to ~/code/bin/csl
```

## Configure

Create `~/.config/csl/config.yaml` listing the directories that contain your git checkouts:

```yaml
dirs:
  - ~/code/src/github.com
  - ~/workspace

# Only index repos whose git remote matches one of these hosts.
# Repos with no remote or a non-matching host are skipped.
# Omit the section entirely to index all discovered repos.
index:
  hosts:
    - github.com
    - git.example.com

# Post-merge reindex hook (opt-in).
hooks:
  post_merge:
    enabled: false
    # Repos to skip — matched by absolute path or org/repo name.
    exclude:
      - ~/workspace/large-monorepo
```

`csl` walks each directory concurrently and records every directory whose immediate child is `.git`. Nested repos are not descended into once a `.git` is found. When `index.hosts` is set, only repos whose origin remote matches a listed host are indexed — use this to avoid indexing repos covered by another search tool.

## Use the CLI

```sh
csl repo --list                     # list every indexed repo
csl search "func Walk"              # search all indexed repos
csl search "TODO" --repo myrepo     # filter to one repo
csl search "fmt\.Errorf" --lang go --output-mode content -C 3
csl hybrid "retry failed HTTP requests"  # fuse lexical + semantic (RRF)
csl count "TODO" --group-by repo    # cross-repo tally
csl hooks install                   # auto-reindex on `git pull` (opt-in)
csl doctor                          # check index + daemon health
```

First search in a fresh checkout triggers an initial index build. Subsequent searches are served by the daemon and return in hundreds of milliseconds.

## Web UI

```sh
csl web --port 7424     # serve the search UI on http://localhost:7424 (loopback only)
```

A localhost web UI over the same index: a search box with example queries, an Examples tab with more, and results grouped by repo and file. Each hit links to the file on its git host (`/blob/HEAD/<file>#L<line>`) and can be expanded inline. The UI is backed by a JSON API you can call directly:

```sh
curl 'http://localhost:7424/api/search?q=func%20Walk&mode=files'
curl 'http://localhost:7424/api/read?repo=thismoon&file=internal/cli/root.go&start=1&end=20'
curl 'http://localhost:7424/api/repos'
```

The server binds to `127.0.0.1` only. To run it as a background service, register it with a process manager, e.g. `t-man add --name csl-web -- csl web --port 7424`.

## Register the MCP server

```sh
claude mcp add --scope user csl -- csl mcp
claude mcp list   # expect: csl: csl mcp - ✓ Connected
```

The MCP server exposes eight tools: search, count, query validation, repo lookup/info/pull/reindex, and file read. See [docs/mcp.md](docs/mcp.md) for the per-tool reference.

To skip the per-call permission prompt, add `"mcp__csl__*"` to `permissions.allow` in `~/.claude/settings.json`.

## Add this to your CLAUDE.md

Registering the MCP server makes the tools available, but Claude will still reach for `find`, `ls`, `Glob`, or raw `grep` by default. Paste the snippet below into `~/.claude/CLAUDE.md` (user-level) or a project `CLAUDE.md` so Claude prefers `csl_*` tools for local repo work:

```markdown
## Local Code Search (csl)

Always use the `csl_*` MCP tools for repo discovery, code search, and file reads across local checkouts. Do not use the `csl` CLI directly — the MCP tools and CLI share the same foundation, so if one is down the other will be too.

Available tools:
- **Repo:** `csl_repo_lookup`, `csl_repo_info`, `csl_repo_pull`, `csl_repo_reindex`
- **Search:** `csl_search`, `csl_semantic_search`, `csl_hybrid_search`, `csl_count`, `csl_read`, `csl_query_validate`

### Repo discovery
- Use `csl_repo_lookup` or `csl_repo_info` to find repos. Do not use `find`, `ls`, `Glob`, or shell to manually search for repo directories.
- `csl_repo_info` returns git health (branch, dirty files, index staleness, suggested action). Call it before starting work on a repo to decide whether to commit, stash, pull, or reindex.
- `csl_repo_lookup` returns `remote` and `host` fields — use them to branch behavior per git host when needed.
- If lookup returns empty, the repo is not checked out locally — say so, don't guess paths.
- Use `csl_repo_pull` before creating branches on repos that may be behind (it has safety checks for dirty state).
- Use `csl_repo_reindex` after significant changes so `csl_search` results stay current.

### Query syntax

zoekt queries look like grep but have important differences:

- **AND is strict:** space-separated terms must ALL appear in the SAME file. Use 1-2 terms and narrow with filters, not 3+ chained terms.
- **OR:** use `|` with no spaces (`foo|bar`) or lowercase `or`. Uppercase `OR` is a literal string. Spaces around `|` break it.
- **Filters:** `repo:name` (not `r:`), `f:\.go$` (not `file:`), `lang:go`, `-term` (NOT).
- **File filters are regex**, not glob: `f:.*\.go$` not `f:*.go`.
- **Exact phrase:** `"foo bar"` requires that exact string on one line. For proximity, use regex: `foo.*bar`.
- **Validate:** call `csl_query_validate` to see how zoekt parsed your query — especially useful when you get zero results.

Common grep-to-zoekt translations:

| grep | zoekt |
|------|-------|
| `grep -r "foo"` | `foo` |
| `grep --include="*.go"` | `f:.*\.go$` |
| `grep -v test` | `-test` |
| `grep -E "foo\|bar"` | `foo\|bar` |
| `find . -name "*.go" \| xargs grep foo` | `foo f:.*\.go$` |

### Multi-repo awareness
When switching working directory to a different git repo, read that repo's `CLAUDE.md` (and `.claude/CLAUDE.md` if present) before making changes. Per-repo instructions take precedence over this global file for repo-specific concerns (build commands, conventions, test frameworks).
```

## Docs

- [Getting started](docs/getting-started.md) — install, configure, first search.
- [Configuration](docs/configuration.md) — config file, paths, environment.
- [CLI reference](docs/cli.md) — every subcommand and flag.
- [Web UI](docs/web.md) — `csl web` browser UI and JSON API.
- [Hooks](docs/hooks.md) — auto-reindex on `git pull` via `csl hooks install`.
- [MCP server reference](docs/mcp.md) — `csl mcp` tool reference for Claude Code.
- [Architecture](docs/architecture.md) — how the daemon, index, and MCP adapter fit together.

## License

MIT. See [LICENSE](LICENSE).
