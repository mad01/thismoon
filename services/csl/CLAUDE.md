# csl, local code search daemon and MCP server backed by zoekt

Indexes local git repos and exposes them over a stdio MCP interface, a CLI,
and a localhost web UI. Imported from `github.com/mad01/code-search-local`;
the name is the source repo's initialism.

## Module layout

```
cmd/csl/           entrypoint
internal/
  cli/             Cobra commands (web, mcp, search, count, query, read,
                    repo, index, semantic, hybrid, sync, hooks, doctor,
                    config, version)
  cslignore/       loads a repo's .cslignore file into compiled glob patterns,
                    shared by the lexical and semantic indexers
  daemon/          search daemon lifecycle (socket, pid, log)
    proto/         gRPC service definition + generated code for the daemon
                    RPC (search.proto, search.pb.go, search_grpc.pb.go)
  search/          zoekt indexer/searcher wrappers
  semantic/        text embedding + code chunking for vector search
  hybrid/          Reciprocal Rank Fusion of lexical + semantic results
  queue/           reindex queue drained by `csl sync` / `csl index --drain`
  syncer/          the sync engine (pull all + reindex changed) shared by
                    `csl sync` and the background refresh, incl. the sync lock
  refresh/         background refresh loop + status inside `csl web`
  repo/            repo discovery + host routing
    config/        config.yaml loading + path resolution (--config, CSL_CONFIG,
                    XDG config dir)
    finder/        concurrent filesystem walk + remote URL parsing
  mcpserver/       MCP stdio server wiring (csl_* tools)
  selfcheck/       the doctor check list, shared by `csl doctor` and the
                    csl_doctor MCP tool (internal/cli already imports
                    mcpserver, so the checks cannot live there)
  web/             HTTP shell + embedded frontend (assets/index.html,
                    assets/static/app.js, app.css)
Makefile
```

Part of the thismoon single Go module. There is no go.mod here: packages live
under `github.com/mad01/thismoon/services/csl/...`.

## How it works

The CLI, `csl mcp`, `csl web`, and the search daemon are all thin shells over
the same internal packages (`internal/search`, `internal/repo/finder`,
`internal/daemon`); no logic is duplicated between them.

**The daemon serves queries when it's running, with an in-process fallback.**
Every query path (CLI `search`/`count`, MCP tool call, web API request) calls
`daemon.EnsureDaemon()`, which pings the daemon's Unix socket and, if nothing
answers, forks `csl search --serve` as a detached background process
(`daemon.StartBackground()`). The daemon opens the zoekt shards once and
keeps them mmap'd across calls, so repeat queries in a session are fast. If
the daemon is unreachable or the RPC fails, the caller opens shards
in-process for that one query instead. The daemon idle-exits after 10
minutes by default (`daemon.idle_timeout_minutes` in config), or on
`SIGINT`/`SIGTERM`/`csl search --stop`.

**Index freshness.** Each repo's fingerprint is
`sha256(HEAD + "\n" + branch + "\n" + git status --porcelain)`, compared
against the one stored in `state.json`. A mismatch marks the repo stale.
`csl search` re-indexes stale repos in a background goroutine after
returning results, so the current query is fast and the next one reflects
the latest state; `csl sync` and `csl index` do it synchronously up front.

**Hybrid search (RRF).** `csl hybrid` / `csl_hybrid_search` run the lexical
(zoekt) and semantic (vector) backends independently, collapse each to one
entry per `(repo, path)`, and fuse the two ranked lists with Reciprocal Rank
Fusion: `score = Σ 1/(k + rank)` summed over whichever backend(s) a file
appears in (`internal/hybrid/fuse.go`, default `k=60`). Zoekt exposes no
numeric relevance score, so fusion is rank-only. When the semantic index
isn't built, `Fuse` receives an empty semantic list and results degrade to
lexical-only with `semantic_available=false`.

### zoekt query pitfalls

Query-writing pitfalls (AND/OR semantics, filter prefixes, regex escaping)
live in `operating.md` under "failure modes"; `csl_query_validate` / `csl
query` shows the parsed tree whenever a query returns unexpected results.

## Data model & storage

`config.yaml` lives in the config directory (see Configuration). Everything
csl writes lives in the state directory, resolved by `csl.StateDir()`
(`services/csl/paths.go`): `$XDG_STATE_HOME/csl`, or `~/.local/state/csl`
when that is unset. **An install whose `~/.config/csl` holds a
`search-index/` keeps using it for state** — that directory is the probe
`confdir.StateDir` tests (`LegacyStateProbe`), so upgrading moves no files
and re-indexes nothing. The probe is deliberate: the fleet recipe symlinks
config.yaml into `~/.config/csl` everywhere, so testing for the directory
itself would strand every provisioned machine on the pre-split path. Every
state path goes through that one resolver; do not join `.config/csl` by
hand.

- **`search-index/`**: the lexical index. `state.json` holds each repo's fingerprint, HEAD, branch, dirty flag, and `indexed_at`; one or more `<shard-hash>.zoekt` shard files sit alongside it per repo. `search-index/.csl-sync.lock` is the cross-process sync lock: `csl sync` and the `csl web` background refresh take it for the whole pull + index phase, and the ad-hoc index writers (`csl search`'s foreground and background reindex, the web fallback's first build, `csl index --repo`) hold it or step aside, so no two writers ever overlap on shards or `state.json`.
- **`semantic-index/`**: per-repo vector stores. Embeddings come from a local Ollama server (jina-code-v2 by default, overridable via `semantic.embed_model`/`semantic.dim`/`semantic.ollama_url`); no model files live on disk here.
- **`search-daemon.sock`**, **`search-daemon.pid`**, **`search-daemon.log`**: the search daemon's Unix socket, PID file, and rotated log (`lumberjack`, 5 MB / 1 backup).
- **`reindex.queue`**: repo paths appended by the suspenders `csl-reindex` post-merge hook after an ad-hoc `git pull`, drained by `csl sync` or `csl index --drain`.

`make install` copies the binary to `$PREFIX/csl`, defaulting to `~/code/bin`.

## Build / install / test

**Needs cgo.** Code chunking compiles tree-sitter grammars
(`smacker/go-tree-sitter`) via cgo with no build-tag opt-out. Release
artifacts build natively on a macOS arm64 runner with `CGO_ENABLED=1`
(the grammars compile statically into the binary, so the tarball is
self-contained); the fleet still installs from source via ralph. Embedding
needs no native libs — it goes over HTTP to a local Ollama server
(semantic/hybrid features only; lexical search has no Ollama dependency).

```bash
make build    # ./csl binary (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to $PREFIX/csl (default ~/code/bin) + codesign
make test     # go test -timeout 120s ./... (unit tests use a fake embedder, no ollama needed)
make lint     # golangci-lint run ./...
```

## Configuration

Config resolution: `--config` (a persistent flag), else `CSL_CONFIG`, else
`config.yaml` in the XDG config dir (`$XDG_CONFIG_HOME/csl`, else
`~/.config/csl`). The cobra root pins the resolved path via
`config.SetPath` in `PersistentPreRunE`, so the CLI, the MCP server, and the
search daemon started from a process all read the same file — a subcommand
that adds its own `PersistentPreRunE` would silently break that. `csl config`
prints the path, whether it loaded, and the settings in effect. Full
reference: `config.md`.

**Zero-config first run.** `Load` returns defaults for a missing file
(`Loaded` false, `Path` set) and an error only for one that exists and cannot
be parsed, so no command needs its own missing-file branch. Commands report
the resulting empty repo list through `cfg.EmptyResultHint()`, which names the
file to create; when a config IS loaded and still yields nothing, that stays an
error. In `csl doctor` the same case is a `doctor.Skip`, so it renders as
`ok config-loads (...)` and reaches the `csl_doctor` tool as a `skipped`
check carrying the path.

Key sections:

- **`dirs`**: directories to walk for git repos.
- **`index.hosts`**: allowlist of git remote hosts. Only repos whose origin remote matches a listed host are indexed. Omit to index all repos.
- **`hooks.post_merge.exclude`**: repos to skip during `csl sync` / index (by absolute path or org/repo name). `hooks.post_merge.enabled` gates the deprecated `csl hooks install` (see Commands); the exclude list itself is still live and shared with `csl sync`.
- **`sync.concurrency`**: parallel pull workers for `csl sync` (default 8).
- **`semantic.enabled`**: whether the search daemon loads the semantic index and embedder at startup. Off by default; lexical search works either way.
- **`semantic.sync`**: whether `csl sync` also re-embeds changed repos after the lexical reindex (best-effort, never fails the sync). Off by default.
- **`semantic.ollama_url` / `semantic.embed_model` / `semantic.dim`**: the Ollama server and embedding model (defaults: `http://localhost:11434`, `unclemusclez/jina-embeddings-v2-base-code:f16`, 768). Per-machine — a smaller machine can point at a smaller model. Changing model or dim triggers a full re-embed on the next index run.
- **`daemon.idle_timeout_minutes`**: how long the search daemon stays alive with no queries (default 10).
- **`refresh.enabled` / `refresh.interval_minutes`**: the background index refresh in `csl web` — a periodic run of the same engine as `csl sync`, sharing its lock. Default enabled, every 15 minutes; manual refresh from the `/refresh` page works even when disabled.
- **`web.base_url`**: where the csl web UI is reachable, used by `csl_show_file` to build the links it opens. Unset derives it from the resolved port (`CSL_PORT`, else 7424) via `csl.BaseURLForPort`; set it to `http://csl.this` when fronted by d-man. `csl web --port` only moves the process it starts, so a permanent move needs `CSL_PORT` or this key.

A repo can also carry a `.cslignore` at its root — one glob per line, `#` comments, trailing `/` for whole trees, leading `/` to anchor at the repo root. It filters both indexes (lexical zoekt and semantic); check its effect with `csl semantic files --skipped <path>`.

`csl sync` discovers repos via `FilteredWalk(cfg.Dirs, cfg.Index.Hosts)`, pulls them (ff-only), and reindexes any that changed. Newly discovered repos that have no entry in `state.json` are also indexed on first sync.

## HTTP API

Owned by `csl web --port 7424` (default port 7424; binds `127.0.0.1` only).
Typically run as a background service: `t-man add --name csl-web -- csl web --port 7424`.

- `GET /`: the search UI (embedded `index.html`).
- `GET /health`: the repo-health page (fleet git state, backed by `/api/repo_health`).
- `GET /refresh`: the index-refresh page — background loop state, per-repo last refresh outcome and indexed-at, manual refresh buttons.
- `GET /file?repo=&file=&start=&end=`: the file-view page — renders a file section like a search match, with expand-to-full-file and copy-path controls. The URL carries all state; `csl_show_file` builds and opens these links.
- `GET /api/search?q=&mode=files_with_matches|content&repo=&lang=&file=&case=yes&limit=&context=`: lexical search, JSON.
- `GET /api/semantic_search?q=&repo=&lang=&k=&expand=`: semantic search, JSON.
- `GET /api/hybrid_search?q=&repo=&lang=&limit=&rrf_k=&expand=`: hybrid (RRF) search, JSON.
- `GET /api/read?repo=&file=&start=&end=`: read a line range from a repo file; the response also carries `localPath`, `fileURL`, `totalLines`, and `truncated` for the file-view page.
- `GET /api/repos`: list discovered repos.
- `GET /api/repo_health?all=`: fleet git-health sweep (dirty counts, ahead/behind upstream, suggested action per repo); only repos needing attention unless `all=true`.
- `GET /api/refresh_status`: background refresh loop state plus per-repo last refresh outcome and `indexed_at`.
- `POST /api/refresh?repo=`: queue a manual refresh (everything, or one repo by `org/repo` name); 202 on queue, 409 while one is already running.
- `GET /healthz`: health check.
- `GET /version`: the four-key build metadata object (`version`, `commit`, `tag`, `build_time`), the consumer `/version` contract every webkit-mounted tool implements (see Shared UI: webkit).
- `GET /webkit/*`: shared UI assets, mounted via `webkit.Mount(mux)`.
- `GET /assets/*`: csl's own static assets (`app.css`, `app.js`).

All handlers run the same daemon-first-then-fallback search path as the CLI and MCP server.

## Commands

CLI subcommands beyond `web` and `mcp` (see HTTP API and MCP tools above/below):

- **`csl search <pattern>`**: search across all indexed repos. `--repo/-r`, `--lang/-l`, `--file/-f`, `--limit` (50), `--output-mode/-o` (files_with_matches\|content), `--context-lines/-C`, `--case-sensitive`, `--reindex` (synchronous re-index first), `--json`, `--toon`. `--serve` starts the search daemon in the foreground (there is no separate `csl serve`); `--stop` stops it.
- **`csl count <pattern>`**: count matches across indexed repos. `--repo/-r`, `--lang/-l`, `--group-by` (repo\|language), `--json`.
- **`csl query <pattern>`**: validate and parse a zoekt query without running a search. `--json`.
- **`csl read <file> --repo <name>`**: read a file from a repo with line numbers. `--repo/-r` (required), `--start-line`, `--end-line`, `--json`.
- **`csl repo [query]`**: interactive fuzzy-finder over discovered repos. With a query, prints the single matching repo's path (case-insensitive substring on org/repo; errors on zero or multiple matches); a query also filters `--list` output. `--list` (non-interactive), `--json`/`--toon` (imply `--list`).
- **`csl doctor`**: run the self-checks, one ok/FAIL line per check — config (parses, and sets at least one dir when the file exists; no file at all passes with a note naming the path to create), state file, index freshness, shard integrity, search-server responsiveness, plus web-only reachability and version-skew probes against the effective web base URL (search works with both failing). `--repair` (reset a corrupt state file). The list lives in `internal/selfcheck` and is also served as the `csl_doctor` MCP tool.
- **`csl config`**: print which config file csl reads, whether it loaded, and the settings in effect once defaults are applied. `--help` carries an annotated reference of every key and of the `CSL_*` environment variables.
- **`csl index`**: manage the search index; by default re-indexes only stale repos. `--all` (full lexical + semantic), `--lexical-all`, `--semantic` (also build the semantic index), `--semantic-all` (semantic-only rebuild; needs Ollama running with the model pulled), `--status`, `--repair` (validate shards, drop corrupted ones), `--clean` (delete the index dir), `--drain` (batch-index repos from `reindex.queue`), `--repo <path>` (single repo), `--json`.
- **`csl semantic <query>`**: search by meaning via vector embeddings. `--repo`, `--lang`, `--k` (10), `--expand`, `--json`. Requires `csl index --semantic-all` first.
- **`csl semantic files [path]`**: classify every tracked file under a path with the indexer's skip rules, without embedding. Default output lists the files that would be embedded; `--skipped` lists filtered files with reasons; `--json` dumps every decision.
- **`csl hybrid <query>`**: lexical + semantic, RRF-fused. `--repo/-r`, `--lang/-l`, `--limit` (50), `--rrf-k` (60), `--expand`, `--json`.
- **`csl sync`**: pull all repos (parallel, ff-only) and batch-reindex the changed ones in one process, one `state.json` write. `--concurrency` (0 = config default, fallback 8), `--dry-run`. Skips repos on a non-default branch, in detached HEAD, with a dirty tree, without a remote, or matching `hooks.post_merge.exclude`. Also drains `reindex.queue` and, when `semantic.sync: true`, re-embeds changed repos. Fails fast (instead of racing) when the background refresh in `csl web` — or another sync — holds the sync lock; the engine is shared with that refresh via `internal/syncer`.
- **`csl hooks`**: **deprecated.** csl no longer manages post-merge hooks; suspenders is the single git-hook manager, feeding `reindex.queue` via a `csl-reindex` post_merge entry that csl still drains (`csl sync` / `csl index --drain`). `csl hooks install` prints a deprecation notice and still writes the legacy hook when `hooks.post_merge.enabled` is set. `csl hooks uninstall` removes any csl-managed post-merge hook; `csl hooks status` (`--json`) reports per-repo hook state. Migration: run `csl hooks uninstall`, then add the `csl-reindex` entry to suspenders' `post_merge` config.
- **`csl version`**: print the bare version token (the git commit built from). `-o/--output text|json`; `-o json` prints the full build metadata object (`version`, `commit`, `tag`, `build_time`).

## MCP tools

`csl mcp` starts the MCP stdio server and registers fifteen `csl_*` tools
(`internal/mcpserver.New()`). Handlers reuse the same daemon-first-then-fallback
path as the CLI, so zoekt shards stay mmap'd across calls in a session.

**Repo:**
- `csl_repo_lookup(name)` → `{matches: [{name, path, remote?, host?}]}`. Resolves a repo name (case-insensitive regex/substring) to its local checkout path; an empty `matches` means the repo isn't checked out locally, so don't guess a path.
- `csl_repo_info(name)` → `{matches: [{..., branch, dirty, modified_files, untracked_files, index_stale, indexed_at, action}]}`. Reports git and index health; `action` is one of `ready`, `commit_or_stash`, `pull_recommended`, `needs_reindex`. Call before creating branches or making changes.
- `csl_repo_health(all?)` → `{total, attention, repos: [{name, path, host?, branch?, dirty, modified_files, untracked_files, ahead, behind, has_upstream, action, error?}]}`. Fleet-wide git-health sweep: which checkouts hold uncommitted or unpushed work. `ahead`/`behind` compare against the last-fetched upstream (no network fetch). `action` is one of `commit_or_stash`, `diverged`, `push_recommended`, `pull_recommended`, `no_upstream`, `detached_head`, `error`, `ready`; default returns only repos needing attention, `all=true` for every repo. Use before a machine switch or as a hygiene sweep.
- `csl_repo_pull(name, force?)` → `{name, path, branch, updated, warning?, old_head?, new_head?}`. Runs `git pull --ff-only`; warns (and no-ops) on a dirty tree or detached HEAD unless `force=true`.
- `csl_repo_reindex(name)` → `{name, path, reindexed, duration}`. Blocking reindex of one repo.

**Search:**
- `csl_search(query, repo?, lang?, file?, output_mode?, context_lines?, limit?, case_sensitive?)` → `{output_mode, files[]|lines[], total, truncated, total_available?}`. Uses zoekt query syntax (see zoekt query pitfalls). Defaults: `limit` 50, `output_mode` files_with_matches; content mode caps at 300 lines per call.
- `csl_count(query, repo?, lang?, group_by?)` → `{total, groups[]}`. Same query syntax as `csl_search`; `group_by` is `repo` or `language`.
- `csl_query_validate(query)` → `{valid, parsed?, error?, hint?}`. Parse-tree debug for zero-result or unexpected-result queries.

**Semantic and hybrid:**
- `csl_semantic_search(query, repo?, lang?, k?, expand?)` → `{available, hits[]?, note?}`. Meaning-based search over vector embeddings; `hits[]` carries `{repo, path, lang?, kind?, start_line, end_line, score, snippet?}`. `repo` here is a case-sensitive substring, not a regex, unlike every other repo param. Returns `available=false` with a `note` when the semantic index isn't built (`csl index --semantic-all`).
- `csl_hybrid_search(query, repo?, lang?, limit?, rrf_k?, expand?)` → `{semantic_available, hits[]?, note?}`. Lexical + semantic fused by RRF; `hits[]` carries both lexical (`lex_rank`, `lex_line`, `lex_text`) and semantic (`sem_rank`, `sem_start`, `sem_end`, `sem_score`, `snippet`) evidence per file. Degrades to lexical-only when semantic is unavailable.

**Read and info:**
- `csl_read(repo, file, start_line?, end_line?)` → `{repo, path, lines[], total_lines, truncated}`. Reads a file by repo name and relative path; caps output at 500 lines unless the caller sets a range.
- `csl_show_file(repo, file, start_line?, end_line?, no_open?)` → `{url, repo, file, local_path, opened, warning?}`. Shows a file section to the USER: builds a `/file` deep link into the csl web UI and opens it in the browser (`no_open=true` to just get the URL). The page renders the section like a search match with expand-to-full-file and copy-path controls, reading live from disk — `csl web` must be running. For reading content yourself, use `csl_read`.
- `csl_ls(repo, path?, glob?, recursive?)` → `{repo, path, entries[], total, truncated, total_available?}`. Lists files/dirs in a repo (glob matches base names; `recursive` returns files only, no dirs); caps at 500 entries.
- `csl_doctor()` → `{ok, checks: [{name, status, detail?}]}`. Runs the same checks as `csl doctor` (config, state file, index freshness, shard integrity, search server, web reachability and version skew) and returns them as JSON, for a client that can call a tool but has no shell. A machine with no config file reports `config-loads` as `skipped` with the path to create in `detail` — a pass with something to say, not a failure. Read-only: never repairs. Distinct from `csl_repo_health`, which is about the indexed repos rather than csl itself.
- `csl_index_info()` → `{repos_indexed, dirty_repos, shards, corrupt_shards, index_size_bytes, newest_indexed_at?, oldest_indexed_at?, daemon_running, semantic: {built, stores, chunks, model_present}}`. Index-wide health in one call; reads state from disk and pings the daemon (no repo scan, sub-second).

## Shared UI: webkit

The chrome (header, theme toggle, font/size/fixation controls) comes from
**`github.com/mad01/thismoon/webkit`**, the in-module package at the repo root
(`webkit/`) that embeds compiled TypeScript/CSS web components. csl compiles
against the webkit committed beside it; there is no version pin. Do NOT re-add
palette, topbar, or theme CSS locally; those live in webkit only.

### How webkit is mounted

```go
import "github.com/mad01/thismoon/webkit"
webkit.Mount(mux) // registers GET /webkit/ (webkit.css, webkit.js, boot.js, version)
```

The FOUC guard is the extracted `/webkit/boot.js`, loaded before the
stylesheet and before `webkit.js`:

```html
<head>
  <script src="/webkit/boot.js"></script>
  <link href="/webkit/webkit.css" rel="stylesheet">
</head>
```

### Header markup (csl)

`index.html` uses:

```html
<wk-header brand="csl·search">
  <a data-nav class="active" href="/">Search</a>
  <a data-nav href="/health">Health</a>
</wk-header>
```

The health and file pages (`health.html`, `file.html`) carry the same header
with the matching link marked `active`; page-specific logic lives in
`static/health.js` / `static/file.js`, with the clipboard helpers shared via
`static/common.js`.

`webkit.js` injects the full control set (font · fixation · size ± · reload · theme)
automatically; don't add those controls manually.

### Per-repo changes

- Header markup lives in `internal/web/assets/index.html`.
- webkit components csl uses:
  - `<wk-header brand="csl·search">` with a `[data-nav]` Search link
  - `<wk-page-header>`, `<wk-title>`, `<wk-subtitle>` for the hero block
  - `<wk-search>` with a leading `<svg>` icon for the main query input
  - `<wk-seg>` for the Lexical / Semantic / Hybrid search-kind toggle and, separately, the Files / Matches view toggle
  - `<wk-badge variant="filter" [active]>` for active filter pills
  - `<wk-table>` for tabular result views
- csl-bespoke CSS (kept in `internal/web/assets/static/app.css`, not part of webkit): result rendering
  (`.repo-group`, `.file-group`, `.line`, `.match`), result quick filters
  (`.result-facets`, `.facet-count`), status row with view toggle
  (`.status-row`, `.status`), pagination
  (`.load-more`), the landing-page example guide shown in the empty state
  (`#examples`, `.ex-section`, `.ex-q`, `.qhl`; data and render logic in `app.js`),
  and zoekt query syntax highlighting on both the search input overlay and the
  example queries (`.search-box`, `.search-hl`, `.qhl`, `.t-*` token classes;
  tokenizer in `app.js`).
- Theme changes fire `document` event `wk-themechange` with `detail.theme`. Listen there (not an `onThemeChange` callback) if a page needs to redraw when the theme flips.
- `[data-nav]` children of `<wk-header>` become nav links; `[data-extra]`
  children become app-specific buttons in the controls area.

### Version check

`GET /version` on csl's own HTTP service returns the four-key build metadata
object (`version`, `commit`, `tag`, `build_time`, every key present and `""`
when unknown) from `github.com/mad01/thismoon/buildinfo`, the consumer contract
every webkit-mounted tool implements (`internal/web/server.go` wires
`s.info.Handler()`). It is separate from `GET /webkit/version`, which reports
the embedded webkit asset hash and is what `webkit.js` polls every ~5s to
auto-reload on a CSS/JS change.

## Gotchas

- **`app.js` no longer owns theme.** `webkit.js` + `<wk-header>` handle theme
  toggling and persistence entirely; `app.js` shouldn't duplicate that logic.
- The MCP server (`csl mcp`) is registered in
  the consuming repo's `recipes/claude-mcp/servers.json` and runs via t-man; the web UI is
  `csl web --port 7424` (the `csl-web` t-man agent). There is no `csl serve`:
  `--serve` is a flag on `csl search` that runs the search daemon in the foreground.
- **`csl hooks install` is deprecated.** suspenders now owns post-merge git
  hooks; don't reintroduce csl-managed hooks in a repo. See Commands.
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `csl docs`): search-server fallback behavior, empty-result
  triage, stale-index detection, semantic availability, version-skew checks.
  Keep those facts there, not here.
- **Ollama model lifecycle.** Bulk semantic index runs unload the embedding
  model when they finish; interactive queries keep it warm for 20 minutes.
  Pull it with `ollama pull unclemusclez/jina-embeddings-v2-base-code:f16`
  (the default; per-machine config may point elsewhere).
- **Version probe.** `GET /version` and `csl version -o json` both return the
  shared four-key build metadata object (`version`, `commit`, `tag`,
  `build_time`, every key present and `""` when unknown) from
  `github.com/mad01/thismoon/buildinfo`, the cross-tool convention `ralph` and
  `status` use to probe the build a sibling tool is running. Plain
  `csl version` stays a bare token. Distinct from `GET /webkit/version` (see
  Shared UI: webkit above), which reports the embedded webkit asset hash.

## See also

- Recipe: `recipes/csl/recipe.toml` (this repo)
- Shared UI package: `webkit/` at the repo root
- Deep-dive docs (not part of this pass): `docs/architecture.md`, `docs/mcp.md`, `docs/cli.md`, `docs/configuration.md`, `docs/getting-started.md`, `docs/semantic.md`, `docs/web.md`, `docs/hooks.md`
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json`
  (entry `csl`, command `csl mcp`), machine-private wiring (docs/adr/0006)
- Route: the consuming repo's `recipes/d-man/routes.toml` overlay
  (`csl` → `csl.this`; docs/adr/0006), machine-private wiring
