# Configuration

All `csl` state lives under `~/.config/csl/`. This page documents every file the tool reads or writes.

## Config file

Path: `~/.config/csl/config.yaml`.

Loaded by the CLI and the MCP server on every invocation that needs to discover repos (`search`, `count`, `read`, `repo`, `doctor`, `index`, and most `csl_*` MCP tools).

### Schema

| Field | Type | Required | Description |
|---|---|---|---|
| `dirs` | list of strings | yes | Root directories that contain your git checkouts. `csl` walks each recursively and records any directory whose child is `.git`. Tildes are expanded. Missing paths are skipped. |
| `index.hosts` | list of strings | no (default `[]`) | Allowlist of git hostnames. When non-empty, only repos whose remote origin hostname matches one of the listed values are indexed. An empty list means all discovered repos are included (no filtering). Repos with no remote are excluded whenever the list is non-empty. |
| `hooks.post_merge.enabled` | bool | no (default `false`) | Master switch for the `csl hooks install` post-merge hook installer. See [hooks reference](hooks.md). |
| `hooks.post_merge.exclude` | list of strings | no (default `[]`) | Repos to skip when installing hooks. Each entry matches against the repo's absolute path or its `org/repo` name. Tildes are expanded. |
| `semantic.enabled` | bool | no (default `false`) | Whether the search daemon connects the embedder and loads the vector index at startup. When `false` the daemon serves lexical search only; the in-process `csl semantic` CLI, the `csl_semantic_search` MCP tool, and the web toggle still answer ad-hoc queries. See [semantic search](#semantic-search). |
| `semantic.sync` | bool | no (default `false`) | Whether `csl sync` also re-embeds the changed files of changed repos after the lexical reindex. When `false`, embeddings refresh only via `csl index --semantic`. |
| `semantic.ollama_url` | string | no (default `http://localhost:11434`) | Base URL of the Ollama server that serves the embedding model. |
| `semantic.embed_model` | string | no (default `qwen3-embedding:0.6b`) | Ollama embedding model. Must be pulled (`ollama pull`). Changing it triggers a full re-embed on the next index run. |
| `semantic.dim` | int | no (default `1024`) | Vector dimensionality of `embed_model`. Must match the model. |
| `sync.concurrency` | int | no (default `8`) | Parallel `git pull` workers for `csl sync`. `--concurrency` on the command line overrides it. |
| `daemon.idle_timeout_minutes` | int | no (default `10`) | How long the search daemon stays alive with no queries. Higher values keep the zoekt shards and semantic stores warm at the cost of resident memory. |

Every key with its default, in one place:

```yaml
# Required: roots to walk for git repos. Tildes expand; missing paths are
# skipped silently.
dirs:
  - ~/code/src/github.com
  - ~/workspace

# Restrict indexing to specific git hosts. Useful when another search
# backend already covers a subset of your repos. Empty/omitted = index
# every discovered repo; non-empty also drops repos with no remote.
index:
  hosts:
    - github.com
    - githost.example.com

# Repos to keep out of the index entirely (lexical and semantic), matched
# by absolute path or org/repo name. `enabled` only gates the deprecated
# `csl hooks install`; the exclude list itself is always live.
hooks:
  post_merge:
    enabled: false
    exclude:
      - ~/workspace/large-monorepo
      - myorg/big-monorepo

# Semantic (vector) search. All optional; see the section below.
semantic:
  enabled: false                          # daemon loads embedder + vector index
  sync: false                             # `csl sync` also re-embeds changed repos
  ollama_url: http://localhost:11434      # default
  embed_model: qwen3-embedding:0.6b       # default; must be pulled in ollama
  dim: 1024                               # must match embed_model

# `csl sync` pull parallelism.
sync:
  concurrency: 8

# Search daemon idle exit.
daemon:
  idle_timeout_minutes: 10
```

### What you can and can't toggle

- **Lexical search is always on.** It's the core of the tool and has no
  disable switch or external dependency. The index builds automatically on
  the first search and refreshes when repo fingerprints change. You control
  its scope, not its existence: `index.hosts` allowlists by git host,
  `hooks.post_merge.exclude` blocklists individual repos (both lexical and
  semantic indexing respect it).
- **Semantic search is opt-in at three separate levels.** The index only
  exists after an explicit `csl index --semantic-all`; the daemon only loads
  it when `semantic.enabled: true`; and it only auto-refreshes during
  `csl sync` when `semantic.sync: true`. Leave everything off and csl never
  talks to Ollama.
- **Hybrid has no switch of its own.** `csl hybrid` fuses whatever is
  available and degrades to lexical-only when the semantic index isn't
  built.
- **Chunking isn't configurable.** Chunk boundaries and the 6000-character
  budget are compile-time decisions tied to the chunker version; changing
  them means a new binary, which triggers the automatic re-embed. The
  embedding model behind the chunks is config (`semantic.embed_model`).
- **There is no semantic-only repo scoping yet.** The exclude list removes
  a repo from both indexes; you can't currently keep a repo lexical-only.

### Semantic search

Semantic (vector) search runs alongside the lexical zoekt index and is off by default. Embedding goes through a local [Ollama](https://ollama.com) server, so semantic features need Ollama running with the model pulled (`ollama pull qwen3-embedding:0.6b`); lexical search has no Ollama dependency. Build the index with `csl index --semantic-all`.

- `semantic.enabled: true` lets the daemon connect the embedder and load the vector index at startup, so the `csl_semantic_search` MCP tool and the web toggle answer from a warm daemon.
- `semantic.sync: true` makes `csl sync` re-embed changed repos after the lexical reindex. The pass is incremental (only changed files) and best-effort: if Ollama or the model is unavailable, the lexical sync still succeeds. Leave it off to refresh embeddings manually with `csl index --semantic`.
- `semantic.ollama_url`, `semantic.embed_model`, and `semantic.dim` override the embedding backend per machine. Changing the model or its dimensionality drops the existing vector stores and re-embeds everything on the next index run. Interactive queries keep the model warm in Ollama for 20 minutes; bulk index runs unload it when they finish.

```yaml
semantic:
  enabled: true
  sync: false
  # ollama_url: http://localhost:11434   # default
  # embed_model: qwen3-embedding:0.6b    # default; must be pulled in ollama
  # dim: 1024                            # must match embed_model
```

### Discovery rules

- The walker uses 32 worker goroutines with a shared work channel. Each worker reads a directory; if any entry is named `.git`, the parent is recorded as a repo and the walker stops descending.
- Hidden directories (any starting with `.`) are skipped.
- Repo `name` is parsed from `.git/config` under `[remote "origin"]`. Both SSH (`git@host:org/repo.git`) and HTTPS (`https://host/org/repo.git`) forms are supported. When no remote is set, the name falls back to `<parent-dir>/<repo-dir>`.
- Repo `host` is extracted from the remote URL (`github.com`, `githost.example.com`) and surfaced in `csl repo --json` output and the `csl_repo_lookup` MCP tool.

## State paths

All paths below are relative to `~/.config/csl/`.

| Path | Purpose |
|---|---|
| `config.yaml` | The config file above. |
| `search-index/` | Zoekt index directory. Contains `*.zoekt` shard files and `state.json`. |
| `search-index/state.json` | Per-repo fingerprints used to decide which repos need re-indexing. |
| `search-index/.csl-sync.lock` | Lock file guarding against concurrent `csl sync` runs racing on `state.json`. |
| `semantic-index/` | Per-repo vector stores (`<org>_<repo>.gob`), written by `csl index --semantic*`. No model files live here — embedding goes through Ollama. |
| `reindex.queue` | Repo paths appended by the suspenders `csl-reindex` post-merge hook, drained by `csl sync` or `csl index --drain`. |
| `search-daemon.sock` | Unix socket the in-memory gRPC search daemon listens on. |
| `search-daemon.pid` | PID file for the running daemon process. |
| `search-daemon.log` | Daemon stdout/stderr. Rotated by lumberjack at 5 MB with one backup. |

### Index contents

Each repo indexed by `csl` produces one or more `*.zoekt` shard files. The shard name is generated by zoekt; it has no stable mapping to the repo name. `csl doctor` and `csl index --status` surface the human-readable mapping.

`state.json` is written atomically (write-to-temp-then-rename) after each indexing round. Its schema:

```json
{
  "repos": {
    "/absolute/path/to/repo": {
      "fingerprint": "<sha256 hex>",
      "head": "<commit sha>",
      "branch": "main",
      "dirty": false,
      "indexed_at": "2026-04-16T12:03:44Z"
    }
  }
}
```

The fingerprint is `sha256(HEAD + "\n" + branch + "\n" + git status --porcelain)`. Any change to committed state, branch, or working tree produces a new fingerprint, so `csl` knows to re-index that repo.

### Daemon lifecycle

- The daemon auto-starts when any command that needs it fails to reach the socket. It runs as a background process via `csl search --serve`, detached from the launching shell.
- Idle timeout: 10 minutes of no RPCs. Any call (including `Ping`) resets the timer.
- `SIGTERM` and `SIGINT` shut it down cleanly. The gRPC server graceful-stops, then the socket and PID file are removed.
- To start manually: `csl search --serve` (runs in foreground).
- To stop manually: `csl search --stop`.

See [architecture](architecture.md#search-daemon) for the full lifecycle.

## Environment

`csl` reads only two environment variables, both standard:

| Variable | Description |
|---|---|
| `HOME` | Root of config/state paths. Used to build `~/.config/csl/...`. |
| `PATH` | The `EnsureDaemon` helper shells out to `csl search --serve` via `os.Executable()` rather than `PATH`, so daemon start works from any cwd. `git` is looked up on `PATH` for pull/fingerprint operations. |

There are no `CSL_*` environment overrides. File issues if you need one.

## Reset

To fully reset indexing state:

```sh
csl search --stop           # stop the daemon first
csl index --clean           # removes ~/.config/csl/search-index/
csl search "anything"       # rebuilds the index on next search
```

`--clean` removes both the shards and `state.json`, so the next search re-indexes every configured repo. The config file is not touched, and neither is the semantic index — to reset that too, remove `~/.config/csl/semantic-index/` and rebuild with `csl index --semantic-all` (or just rebuild: a model/dim/chunker change re-embeds automatically without the manual delete).
