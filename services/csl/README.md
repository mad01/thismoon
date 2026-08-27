# csl

Local code search over your git checkouts. A CLI, a localhost web UI, and an MCP stdio server for Claude Code, all backed by [zoekt](https://github.com/sourcegraph/zoekt). For people who want fast cross-repo `grep`, and for agents that need to search, resolve repo paths, and read files without shelling out.

`csl` walks the directories you configure, indexes every git repo it finds, and searches them with zoekt's trigram index. You get `grep`-like queries across dozens of checkouts in milliseconds, without shipping your code to a cloud service. A short-lived search daemon spawns on demand from the same binary, keeps zoekt shards mmap'd across queries, and exits after ten minutes of idleness; there is no server to manage separately.

## How it works

The CLI, `csl mcp`, `csl web`, and the search daemon share the same internal search/index code. A query first tries the daemon over a Unix socket (auto-started if not running); if that fails, it falls back to opening the zoekt shards in-process. Index freshness is tracked per repo by a fingerprint of `HEAD` + branch + `git status`, so stale repos get reindexed in the background after results come back. `csl hybrid` / `csl_hybrid_search` add a second backend (vector embeddings over the same repos, searched by meaning) and fuse it with the lexical results by Reciprocal Rank Fusion.

## Install

Needs cgo. Code chunking compiles tree-sitter grammars via cgo with no build-tag opt-out, so `go install .../csl@latest` isn't a supported install path. Release artifacts build natively on a macOS arm64 runner, so the grammars compile statically into the tarball binary; the fleet still installs from source (see `docs/MIGRATED-FROM.md`).

Prebuilt darwin/arm64 tarballs ship on each [`csl/vX.Y.Z` release](https://github.com/mad01/thismoon/releases) with `checksums.txt` and a cosign keyless bundle; verification steps are in `docs/RELEASING.md`. Building from source:

```sh
git clone https://github.com/mad01/thismoon.git
cd thismoon/services/csl
make install   # builds with version embedded, copies to ~/code/bin/csl, codesigns
```

Requires the Go toolchain pinned in the repo's `go.mod` (1.26.2) and `git` on `PATH`. Semantic search additionally needs a running [Ollama](https://ollama.com) with the embedding model pulled (`ollama pull unclemusclez/jina-embeddings-v2-base-code:f16`); lexical search works without it.

Verify:

```sh
csl version
```

## Configuration

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

# Repos to skip during `csl sync` / indexing: matched by absolute
# path or org/repo name.
hooks:
  post_merge:
    exclude:
      - ~/workspace/large-monorepo

sync:
  concurrency: 8       # parallel `csl sync` pull workers (default 8)

semantic:
  enabled: false   # load the semantic index + embedder in the daemon
  sync: false      # also re-embed changed repos on `csl sync` (best-effort)
  # ollama_url: http://localhost:11434   # Ollama server for embeddings
  # embed_model: unclemusclez/jina-embeddings-v2-base-code:f16    # embedding model (must be pulled)
  # dim: 768                             # its vector dimensionality

daemon:
  idle_timeout_minutes: 10   # how long the search daemon stays alive when idle
```

`csl` walks each directory concurrently and records every directory whose immediate child is `.git`; it doesn't descend into nested repos once it finds a `.git`. When `index.hosts` is set, only repos whose origin remote matches a listed host are indexed; use this to avoid indexing repos covered by another search tool.

`hooks.post_merge.enabled` also gates the deprecated `csl hooks install` (see Usage); leave it unset unless you're deliberately using the legacy hook installer.

## Usage

```sh
csl repo --list                          # list every indexed repo
csl search "func Walk"                   # search all indexed repos
csl search "TODO" --repo myrepo          # filter to one repo
csl search "fmt\.Errorf" --lang go --output-mode content -C 3
csl hybrid "retry failed HTTP requests"  # fuse lexical + semantic (RRF)
csl semantic "where do we retry failed requests"  # meaning-based search
csl count "TODO" --group-by repo         # cross-repo tally
csl sync                                 # pull every repo (ff-only), reindex what changed
csl doctor                               # check index + daemon health
csl config                               # which config file is read, and the settings in effect
```

First search in a fresh checkout triggers an initial index build. The daemon serves subsequent searches and returns them in hundreds of milliseconds. Semantic and hybrid search need `csl index --semantic-all` run once first, with Ollama running and the embedding model pulled (`ollama pull unclemusclez/jina-embeddings-v2-base-code:f16`).

`csl hooks install` (legacy per-repo post-merge hook installer) is deprecated: suspenders now owns git hooks; see Configuration.

For a browser UI instead of the CLI:

```sh
csl web --port 7424     # serve the search UI on http://localhost:7424 (loopback only)
```

A localhost web UI over the same index: a search box (lexical/semantic/hybrid) with example queries, an Examples tab with more, and results grouped by repo and file. Each hit links to the file on its git host and can be expanded inline. To run it as a background service, register it with a process manager, e.g. `t-man add --name csl-web -- csl web --port 7424`.

## Endpoints

A JSON API backs the web UI and you can call it directly (`127.0.0.1` only):

```sh
curl 'http://localhost:7424/api/search?q=func%20Walk&mode=files_with_matches'
curl 'http://localhost:7424/api/semantic_search?q=retry%20failed%20requests'
curl 'http://localhost:7424/api/hybrid_search?q=retry%20failed%20requests'
curl 'http://localhost:7424/api/read?repo=thismoon&file=internal/cli/root.go&start=1&end=20'
curl 'http://localhost:7424/api/repos'
curl 'http://localhost:7424/healthz'
curl 'http://localhost:7424/version'
```

## MCP

```sh
claude mcp add --scope user csl -- csl mcp
claude mcp list   # expect: csl: csl mcp - ✓ Connected
```

On a ralph-managed machine, skip the manual command — MCP registration is machine-private wiring that ships from the consuming repo's companion recipe (`docs/adr/0006` at the repo root). Nothing needs to be running first: the search daemon auto-starts on the first query and idles out on its own (see How it works). What does need to exist is `dirs` in `~/.config/csl/config.yaml` (see Configuration) — without at least one directory to walk, there's nothing to index. If a tool call comes back empty or erroring, run `csl doctor`.

The MCP server exposes twelve `csl_*` tools:

- **Repo:** `csl_repo_lookup`, `csl_repo_info`, `csl_repo_pull`, `csl_repo_reindex`
- **Search:** `csl_search`, `csl_count`, `csl_query_validate`
- **Semantic and hybrid:** `csl_semantic_search`, `csl_hybrid_search`
- **Read and info:** `csl_read`, `csl_ls`, `csl_index_info`

See [docs/mcp.md](docs/mcp.md) for the per-tool reference (inputs, return shape, defaults).

To skip the per-call permission prompt, add `"mcp__csl__*"` to `permissions.allow` in `~/.claude/settings.json`.

### Add this to your CLAUDE.md

Registering the MCP server makes the tools available, but Claude will still reach for `find`, `ls`, `Glob`, or raw `grep` by default. Paste the snippet below into `~/.claude/CLAUDE.md` (user-level) or a project `CLAUDE.md` so Claude prefers `csl_*` tools for local repo work:

```markdown
## Local Code Search (csl)

Always use the `csl_*` MCP tools for repo discovery, code search, and file reads across local checkouts. Do not use the `csl` CLI directly: the MCP tools and CLI share the same foundation, so if one is down the other will be too.

Available tools:
- **Repo:** `csl_repo_lookup`, `csl_repo_info`, `csl_repo_pull`, `csl_repo_reindex`
- **Search:** `csl_search`, `csl_semantic_search`, `csl_hybrid_search`, `csl_count`, `csl_read`, `csl_query_validate`
- **Other:** `csl_ls`, `csl_index_info`

### Repo discovery
- Use `csl_repo_lookup` or `csl_repo_info` to find repos. Do not use `find`, `ls`, `Glob`, or shell to manually search for repo directories.
- `csl_repo_info` returns git health (branch, dirty files, index staleness, suggested action). Call it before starting work on a repo to decide whether to commit, stash, pull, or reindex.
- `csl_repo_lookup` returns `remote` and `host` fields; use them to branch behavior per git host when needed.
- If lookup returns empty, the repo is not checked out locally; say so, don't guess paths.
- Use `csl_repo_pull` before creating branches on repos that may be behind (it has safety checks for dirty state).
- Use `csl_repo_reindex` after significant changes so `csl_search` results stay current.

### Query syntax

zoekt queries look like grep but have important differences:

- **AND is strict:** space-separated terms must ALL appear in the SAME file. Use 1-2 terms and narrow with filters, not 3+ chained terms.
- **OR:** use `|` with no spaces (`foo|bar`) or lowercase `or`. Uppercase `OR` is a literal string. Spaces around `|` break it.
- **Filters:** `repo:name` (not `r:`), `f:\.go$` (not `file:`), `lang:go`, `-term` (NOT).
- **File filters are regex**, not glob: `f:.*\.go$` not `f:*.go`.
- **Exact phrase:** `"foo bar"` requires that exact string on one line. For proximity, use regex: `foo.*bar`.
- **Validate:** call `csl_query_validate` to see how zoekt parsed your query. This is especially useful when you get zero results.

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

## Where things live

Everything lives under `~/.config/csl/`:

- `config.yaml`: see Configuration.
- `search-index/`: lexical index (`state.json` per-repo fingerprint/branch/dirty/indexed-at, plus `<shard-hash>.zoekt` shard files).
- `semantic-index/`: per-repo vector stores (embeddings come from Ollama; no model files live here).
- `search-daemon.sock`, `search-daemon.pid`, `search-daemon.log`: the search daemon's socket, PID file, and rotated log.
- `reindex.queue`: repo paths queued by the suspenders `csl-reindex` post-merge hook; drained by `csl sync` / `csl index --drain`.

`make install` puts the binary at `~/code/bin/csl`.

## Develop

```sh
make build    # ./csl binary (fetches native ONNX/tokenizer libs first, macOS arm64)
make test     # go test -timeout 120s ./... (unit tests use a fake embedder, no native libs needed)
make lint     # golangci-lint run ./...
```

Further docs:

- [Getting started](docs/getting-started.md): install, configure, first search.
- [Configuration](docs/configuration.md): config file, paths, environment.
- [CLI reference](docs/cli.md): every subcommand and flag.
- [Web UI](docs/web.md): `csl web` browser UI and JSON API.
- [Semantic search](docs/semantic.md): lexical vs semantic, the vector index, embedding models.
- [Hooks](docs/hooks.md): the deprecated post-merge hook installer and the suspenders migration path.
- [MCP server reference](docs/mcp.md): `csl mcp` tool reference for Claude Code.
- [Architecture](docs/architecture.md): how the daemon, index, and MCP adapter fit together.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference

## License

BSD 3-Clause License - see the LICENSE file at the repository root.
