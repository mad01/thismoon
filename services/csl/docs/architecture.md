# Architecture

How `csl` fits together: what spawns what, where state lives, and how a query reaches the index.

## Overview

```
┌────────────┐        ┌───────────────┐      ┌────────────┐
│ user CLI   │──┐     │ csl mcp (MCP) │◀──JSON-RPC── Claude Code
│ csl search │  │     └───────┬───────┘      └────────────┘
└────────────┘  │             │
                ▼             ▼
         ┌──────────────────────────┐       ┌──────────────┐
         │ internal/daemon (client) │──gRPC─▶│ csl daemon   │
         └──────────────────────────┘       │ (search --serve)
                │ fallback                  └──────┬───────┘
                ▼                                  │
         ┌──────────────────────────┐              │
         │ internal/search (local)  │              │
         └───────┬──────────────────┘              │
                 │                                 │
                 ▼                                 ▼
         ┌──────────────────────────┐       ┌──────────────┐
         │ <state dir>/             │       │ mmap'd zoekt │
         │   search-index/*.zoekt   │       │ shards       │
         └──────────────────────────┘       └──────────────┘
```

The CLI and the MCP server are both thin shells over the same internal packages. The daemon is another thin shell over the same `internal/search` functions, with the searcher held open so shards stay mmap'd between calls.

## Packages

| Package | Role |
|---|---|
| `cmd/csl` | `main.go`; calls `internal/cli.Execute()` |
| `internal/cli` | One file per cobra subcommand. Mostly argument parsing and dispatch into the other packages |
| `internal/repo/config` | Resolves the config path (`--config`, `CSL_CONFIG`, XDG config dir) and parses `config.yaml` |
| `internal/repo/finder` | Concurrent filesystem walk that discovers git repos and parses `[remote "origin"]` URLs |
| `internal/repo/catalogspec` | Reads the catalog descriptor a repo may carry at its root (`catalog-info.yaml`, `service-info.yaml`, or a `.csl-catalog.yaml` pointer) |
| `internal/picker` | csl's own fuzzy-finder picker (`tcell`), drawing colored segments per line for `csl repo` |
| `internal/selfcheck` | The doctor check list, served by both `csl doctor` and the `csl_doctor` MCP tool |
| `internal/search` | Indexing (`IndexRepo`, `IndexRepos`), searching (`Search`, `SearchWith`), counting, query validation, shard integrity |
| `internal/semantic` | Vector search: tree-sitter chunking, embedding via Ollama (`OllamaEmbedder`), per-repo vector stores, cosine ranking |
| `internal/hybrid` | Reciprocal Rank Fusion of the lexical and semantic result lists |
| `internal/daemon` | gRPC server (`Serve`), client helpers (`SearchVia`, `CountVia`, `Ping`, `Shutdown`), lifecycle (`StartBackground`, `EnsureDaemon`, PID file management) |
| `internal/daemon/proto` | Protobuf-generated gRPC types |
| `internal/mcpserver` | MCP tool handlers. Shares the same daemon-first-then-fallback path as `internal/cli` |
| `internal/web` | `csl web` HTTP server: embedded UI assets, JSON API, and a `Service` that shares the same daemon-first-then-fallback search path |

## Request paths

### `csl search` (CLI)

1. `internal/cli/search.go` loads config, walks repos, builds a `SearchOptions`.
2. Calls `daemon.EnsureDaemon()` — if the socket is dead, forks `csl search --serve` as a detached background process and pings until it answers (10 attempts × 100 ms).
3. `daemon.SearchVia()` dials the Unix socket, sends a `SearchRequest`, decodes matches.
4. If the daemon is unreachable or the RPC fails, falls back to `search.Search()` which opens shards in-process, runs the query, closes shards.

Index freshness is checked once per CLI call via `search.CheckStaleness()`. Stale repos are re-indexed in a goroutine after the results are returned, so the user sees results fast and the next call reflects the latest state.

### `csl mcp` → Claude Code tool call

1. Claude Code spawns `csl mcp` as a subprocess and sends an `initialize` JSON-RPC message over stdin.
2. `internal/mcpserver.New()` registers fifteen `csl_*` tools against the `modelcontextprotocol/go-sdk` server.
3. A `tools/call` for `csl_search` lands in `handleSearch`, which follows the same daemon-first-then-fallback pattern as the CLI (minus the stderr progress output).
4. When Claude Code closes the session, stdin EOF causes the server loop to return. The process exits.

No daemon is started by `csl mcp` itself. The daemon is shared: whichever of the CLI or MCP calls first triggers `EnsureDaemon` spawns it; everyone else reuses it.

### `csl web` → browser request

1. `internal/cli/web.go` loads config and builds an `internal/web.Service`, then serves an `http.ServeMux` bound to `127.0.0.1` (loopback only).
2. A page request returns embedded HTML/CSS/JS (`internal/web/assets`, via `go:embed`); the JS calls the JSON API.
3. `GET /api/search` builds a `SearchOptions` and calls `Service.Search()`, which follows the same daemon-first-then-fallback pattern as the CLI. The handler groups the flat matches by repo and file and attaches remote links via `finder.FileURL()`.
4. `GET /api/read` reads a line range from a repo file (path confined to the repo root); `GET /api/repos` lists discovered repos.

Like the MCP server, `csl web` starts no daemon of its own — it reuses the shared daemon through `EnsureDaemon`.

## Search daemon

### Lifecycle

- **Start:** `daemon.StartBackground()` resolves its own executable via `os.Executable()` and launches `csl search --serve` as a detached child (`Setpgid: true`), with stdout/stderr redirected to the log file. The parent does not wait on the child; it returns as soon as `Start` succeeds.
- **Listen:** `Serve()` opens `zoektsearch.NewDirectorySearcher(indexDir)` once, binds a Unix socket at `<state dir>/search-daemon.sock`, writes its PID to `search-daemon.pid`.
- **Serve:** The gRPC server handles `Search`, `Count`, `Validate`, `Ping`, `Shutdown`. Every handler calls `resetIdle()` to reset a 10-minute idle timer.
- **Shut down:** Triggered by `SIGINT`, `SIGTERM`, an RPC `Shutdown`, or the idle timer firing. `grpcServer.GracefulStop()` then the searcher is closed and the socket and PID files are removed.

### Socket discovery

`EnsureDaemon(indexDir, socketPath)` does a one-shot `Ping`, and if that fails, calls `StartBackground()` and polls `Ping` for one second. `ErrDaemonNotRunning` means the caller falls through to in-process search.

### Log rotation

The daemon's log writer is [`lumberjack.Logger`](https://pkg.go.dev/gopkg.in/natefinch/lumberjack.v2) with `MaxSize: 5 MB, MaxBackups: 1`. The Go `log` package is redirected to it. Search progress lines from the daemon end up there, not in the terminal that spawned it.

## Index layout

`<state dir>` is `$XDG_STATE_HOME/csl`, or `~/.local/state/csl` when that
variable is unset — except on installs that predate the config/state split,
where an existing `~/.config/csl` keeps the role. `csl.StateDir()` in
`services/csl/paths.go` is the one place that decides; see
[configuration](configuration.md#state-paths).

```
<state dir>/search-index/
├── state.json                       # per-repo fingerprints
├── <shard-hash>.zoekt               # zoekt shards — one or more per repo
└── ...
```

### Fingerprints

Each repo has one entry in `state.json`:

```json
{
  "fingerprint": "<sha256>",
  "head": "<commit sha>",
  "branch": "main",
  "dirty": false,
  "indexed_at": "2026-04-16T12:03:44Z"
}
```

The fingerprint is `sha256(HEAD + "\n" + branch + "\n" + git status --porcelain)`. `search.CheckStaleness()` diffs the current fingerprint against the stored one; any difference means the shard is out of date.

### Shard validation

`search.ValidateShards()` opens each `*.zoekt` file and calls `index.NewIndexFile` + `index.ReadMetadata`. A failure marks the shard corrupted. `search.RepairIndex()` removes corrupted shards and drops the corresponding entries from `state.json` so the next `csl index` picks those repos up.

### What's indexed

`IndexRepo` walks the working tree (not the git object database), so uncommitted changes are searchable. The walker skips:

- any hidden directory except the repo root (rules out `.git`, `.cache`, `.venv`)
- `node_modules`, `vendor`, `__pycache__`, `build`, `dist`, `target`
- files larger than the zoekt default `SizeMax` (1 MB)
- non-regular files (symlinks, devices, sockets)

## Query flow

`search.buildQueryString()` serializes a `SearchOptions` into a zoekt query string by appending clauses:

```go
opts := SearchOptions{
    Pattern:       "func",
    RepoFilter:    "test/repo",
    FileFilter:    "main",
    Lang:          "go",
    CaseSensitive: true,
}
// produces: "func repo:test/repo file:main lang:go case:yes"
```

That string is parsed by `zoekt/query.Parse`, simplified, and passed to the searcher. Match results include the matching line, line/column, and context lines when requested.

## Semantic index

The vector index lives beside the lexical one:

```
<state dir>/semantic-index/
└── <org>_<repo>.gob      # one store per repo: chunk vectors + metadata
```

Each store records, per file, a content hash and the chunk vectors, plus two
store-wide invariants: the chunker version and the vector dimensionality.
`IndexRepoSemantic` skips any file whose content hash is unchanged, so
re-runs are incremental; a mismatch on either invariant drops the whole
store and re-embeds the repo, so vectors produced under different rules are
never mixed.

### The Ollama boundary, and why

Embedding runs out-of-process: `internal/semantic.OllamaEmbedder` POSTs
chunk batches to an Ollama server's `/api/embed` and gets vectors back.
csl bundles no model and links no inference runtime.

An earlier design embedded in-process through ONNX Runtime with a
compiled-in all-MiniLM-L6-v2 model. It worked, but every property that
mattered was fixed at build time: the model (trained on prose, not code),
its 512-token context (which forced ~900-character chunks that cut
functions mid-body), and a native-library dependency that complicated every
build. Moving the model behind an HTTP boundary inverts all three:

- **The model is per-machine config** (`semantic.ollama_url`,
  `semantic.embed_model`, `semantic.dim`), not a build decision. A laptop
  can run a heavier code-trained model while a constrained machine points
  at a lighter one — same binary.
- **Long-context models fit whole declarations.** The chunk budget is 6000
  characters, and requests pin `num_ctx`/`num_batch` at 8192 tokens so
  nothing is silently truncated (and oversized batches don't crash the
  runner, which Ollama's 2048-token default physical batch does).
- **Ollama owns model lifetime and the GPU.** csl asks for a 20-minute
  `keep_alive` so interactive queries hit a warm model, and explicitly
  unloads after bulk index runs so a rebuild doesn't leave the model
  resident. Other Ollama consumers (other tools, other models) coexist
  under the same scheduler.

The cost is a runtime dependency: semantic indexing and semantic/hybrid
queries need Ollama up with the configured model pulled (`CheckModel`
preflights this and says what to pull). Lexical search never touches
Ollama, so the dependency is scoped to the features that need it.

The daemon loads the vector stores at startup when `semantic.enabled:
true` and serves semantic queries from memory over the same gRPC socket;
without the daemon, callers load stores in-process for the one query. See
[semantic.md](semantic.md) for the chunking/query pipeline and model
selection.

## Hybrid search (RRF)

`csl hybrid` and `csl_hybrid_search` compose the two existing backends (zoekt and the semantic vector index) without modifying the daemon protocol. The daemon already holds both indexes; no new gRPC messages are needed.

Fusion is implemented in `internal/hybrid/fuse.go` (`Fuse([]search.Match, []semantic.Result, k, limit) []FusedHit`). The algorithm:

1. Each backend runs independently and returns a ranked list.
2. Results are collapsed to one entry per `(repo, path)` pair (lexical runs in content mode; the top-ranked line per file is kept).
3. For each file, Reciprocal Rank Fusion (RRF) computes `score = sum of 1/(k+rank)` over whichever lists the file appears in. Files missing from a list contribute nothing from that list.
4. The merged list is sorted by descending fused score and truncated to `limit`.

Fusion is rank-only: zoekt exposes no numeric relevance score, so there is no score normalization step. The default k=60 follows Cormack et al. 2009 and matches the defaults used by Elasticsearch, Weaviate, Qdrant, and Azure AI Search.

The MCP handler lives in `internal/mcpserver/tools_hybrid.go`; the CLI command in `internal/cli/hybrid.go`. When the semantic index is unavailable, `Fuse` receives an empty semantic result list and the output degrades to the lexical results only.

## Repo discovery

`finder.Walk` runs a concurrent BFS over every configured directory with a pool of 32 workers sharing a single work channel. For each directory, one `os.ReadDir` call is made:

- If `.git` is an entry, the directory is recorded as a repo and the walker stops descending.
- Otherwise, every non-hidden child directory is queued.

Repo names come from the `[remote "origin"]` URL in `.git/config`. Both SSH (`git@host:org/repo.git`) and HTTPS (`https://host/org/repo.git`) forms are parsed. If no origin is set, the name is the parent directory plus repo directory, joined by `/`.

This walker is deliberately csl's own rather than the shared [`kit/repofind`](../../../kit/repofind/README.md) package the belt and suspenders guards use: indexing rescans every checkout on the machine often enough that reading `.git/config` directly (no `git` subprocess) matters, and csl also needs the host for grouping. Only `ExpandHome` is shared. Two things outside csl depend on this layer's output shape: the shard files are named `<org>%2F<repo>_v<N>.zoekt`, and belt's `prefer-csl` hint reads the shard directory listing (never a csl process) to decide whether a swept path lies in an indexed repo — so shard naming is a small external contract, not a private detail.

## Testing

Run with:

```sh
make test
```

Which expands to `go test -timeout 30s ./...`. The test suite covers:

- `internal/repo/finder` — walker, remote/host parsing, edge cases (no remote, HTTPS, SSH), catalog-descriptor query matching (`Query.SubstringMatcher`/`RegexMatcher`).
- `internal/repo/catalogspec` — parsing Backstage- and catalog-service-shaped descriptors, apiVersion group and kind matching, the `.csl-catalog.yaml` pointer, malformed-file handling.
- `internal/picker` — fuzzy matching and key handling for the picker.
- `internal/search` — indexing, search with filters, count grouping, query validation, re-indexing after edits, shard build and validation. Includes benchmarks (`BenchmarkIndexRepo`, `BenchmarkSearch`).
- `internal/daemon` — lifecycle (PID read/write, `IsRunning` against dead and live PIDs, `RemoveStale`), gRPC server end-to-end (`TestSearch`, `TestCount`, `TestValidate`, `TestPing`, `TestShutdown`, `TestIdleTimeout`).
- `internal/cli/repo_test.go` — cobra command with fake `HOME`, synthetic git repos, JSON and TOON output.

Tests use `t.TempDir()` and real `git init` subprocesses. There are no mocks.

## Extending

Adding a new MCP tool:

1. Define the typed input/output structs with `jsonschema` tags in `internal/mcpserver/tools_<area>.go`.
2. Register the tool inside the corresponding `register*Tools(s *mcp.Server)` function.
3. Write the handler. Share logic with the CLI by calling into `internal/search`, `internal/daemon`, or `internal/repo/finder`. Don't reimplement.

Adding a new CLI subcommand:

1. Create `internal/cli/<name>.go`. Define a `cobra.Command`, register it in `init()` via `rootCmd.AddCommand(&cmd)`.
2. Keep the command thin. Business logic belongs in `internal/search`, `internal/daemon`, or `internal/repo/*`.
3. Add tests that run the command through `rootCmd.Execute()` with `SetArgs` and capture output with `SetOut`/`SetErr`. See `internal/cli/repo_test.go` for the pattern.
