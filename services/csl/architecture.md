# csl architecture

## Overview

csl is the local code search service: it indexes every git checkout under the
configured source directories with zoekt and answers queries from three
surfaces built from one executable. The `csl` CLI runs one-shot commands,
`csl mcp` serves the `csl_*` tools over stdio, and `csl web --port 7424` binds
a loopback HTTP server (fronted by d-man as `http://csl.this/`, run as a t-man
agent). A fourth process, the background search server (`csl search --serve`),
is forked on demand from the same executable, holds the zoekt shards mmap'd
across queries, and idle-exits after ten minutes. csl's boundary is read-only
apart from `csl sync` / `csl_repo_pull`, which are fast-forward-only pulls.

## Structure

```
cmd/csl/            entrypoint, delegates to internal/cli
internal/
  cli/              one Cobra command per file (search, index, sync, web, mcp, ...)
  daemon/           search-server lifecycle: Unix socket, gRPC client/server,
                    EnsureDaemon/StartBackground, PID file
  search/           zoekt wrappers: IndexRepo(s), Search, Count, staleness checks
  semantic/         tree-sitter chunking, Ollama embedding, per-repo vector stores
  hybrid/           Reciprocal Rank Fusion of lexical and semantic result lists
  queue/            reindex queue drained by csl sync / csl index --drain
  repo/config/      ~/.config/csl/config.yaml loading
  repo/finder/      concurrent filesystem walk, remote-URL parsing, and the
                    name/component/owner/system query matcher
  repo/catalogspec/ reads a repo's root catalog descriptor (catalog-info.yaml,
                    service-info.yaml, or the file .csl-catalog.yaml points at)
  picker/           the colored fuzzy picker behind csl repo (tcell)
  cslignore/        per-repo .cslignore glob filtering, shared by both indexes
  mcpserver/        MCP stdio wiring for the csl_* tools
  notify/           best-effort event emission to the events service
  web/              HTTP server, JSON API, embedded index.html/app.js/app.css
```

All four surfaces are thin shells over the same internal Go packages.
`internal/web` mounts the shared chrome with `webkit.Mount(mux)` and keeps
only csl-specific result rendering in its own embedded assets.

## Data flow

Every query path (CLI `search`/`count`, MCP tool call, web API handler) calls
`daemon.EnsureDaemon()`: it pings the Unix socket and, if nothing answers,
forks `csl search --serve` detached. The query then travels as gRPC over the
socket (`daemon.SearchVia`); if the search server is unreachable the caller
falls back to `search.Search()`, opening shards in-process for that one query.
Freshness rides alongside: each repo's fingerprint (sha256 of HEAD, branch,
and `git status --porcelain`) is compared to `state.json`, and stale repos are
reindexed in a background goroutine after results return.

Indexing is the other half of the split. `csl sync` walks the configured
directories (`finder.FilteredWalk`), pulls repos ff-only in parallel, batch
reindexes the changed ones, and drains `reindex.queue`. Semantic indexing
(`csl index --semantic-all`) chunks source files with tree-sitter, embeds the
chunks over HTTP against a local Ollama server, and writes per-repo vector
stores. `csl hybrid` and `csl_hybrid_search` run both backends, collapse each
to one entry per (repo, path), and fuse the ranked lists with RRF in
`internal/hybrid/fuse.go`; without a semantic index the result degrades to
lexical-only.

## Storage

Everything lives under `~/.config/csl/`:

- `config.yaml`: directories, host allowlist, semantic and sync settings.
- `search-index/`: the lexical index. `state.json` records each repo's
  fingerprint, HEAD, branch, dirty flag, and `indexed_at`; one or more
  `<hash>.zoekt` shard files sit alongside it per repo. `.csl-sync.lock`
  guards concurrent `csl sync` runs.
- `semantic-index/`: one gob-encoded vector store per repo (`<repo>.gob`,
  written atomically via temp file and rename) plus a semantic state file.
  Embeddings come from Ollama at query/index time; no model files land here.
- `search-daemon.sock` / `.pid` / `.log`: the search server's socket, PID
  file, and rotated log.
- `reindex.queue`: repo paths appended by the suspenders post-merge hook,
  drained by `csl sync` or `csl index --drain`.

## Interfaces

The web server exposes `GET /api/search`, `/api/semantic_search`,
`/api/hybrid_search`, `/api/read`, `/api/repos`, `/healthz`, `/version`
(service git sha), `/webkit/*` (shared chrome), and `/assets/*` (csl's own).
The CLI mirrors the same paths: `search`, `count`, `query`, `read`, `repo`,
`doctor`, `index`, `semantic`, `hybrid`, `sync`, plus `web`, `mcp`,
`version`, and the deprecated `hooks`. `csl mcp` registers twelve tools:
`csl_repo_lookup`, `csl_repo_info`, `csl_repo_pull`, `csl_repo_reindex`,
`csl_search`, `csl_count`, `csl_query_validate`, `csl_semantic_search`,
`csl_hybrid_search`, `csl_read`, `csl_ls`, and `csl_index_info`.
`csl_repo_lookup` also takes `component`, `owner`, and `system` filters
(case-insensitive regex, matched against the repo's root catalog descriptor)
and returns those fields on each match alongside `name`, `path`, `remote`,
and `host`; a repo with no descriptor never matches the three catalog
filters. Config is
`~/.config/csl/config.yaml` (dirs, `index.hosts`, `semantic.*`,
`sync.concurrency`, `daemon.idle_timeout_minutes`) plus per-repo `.cslignore`
files and the optional catalog descriptor each repo may carry at its root. A
deeper walkthrough lives in `docs/architecture.md`.
