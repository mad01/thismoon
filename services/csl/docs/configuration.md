| `index.allow_hidden_dirs` | list of strings | no (default `[]`) | Hidden directory names the file walk enters on top of the built-in `.github` and `.claude`. Names only (each starts with a dot, no slashes); `.git` is refused. Files under an allowed hidden directory are indexed only when git tracks them, so gitignored local settings stay out. Every other hidden directory (`.cache`, `.venv`, `.terraform`) stays out of the lexical index. |
# Configuration

`csl` keeps its config file and its state in two different places. This page documents every file the tool reads or writes.

## Config file

Resolved in this order:

1. `--config <path>` — a persistent flag, available on every subcommand.
2. `$CSL_CONFIG`.
3. `config.yaml` in the XDG config directory: `$XDG_CONFIG_HOME/csl` when that variable holds an absolute path, otherwise `~/.config/csl`.

A leading `~` is expanded in both the flag and the environment variable, so they work under launchd where no shell expands them first.

The file is optional. With none, csl runs on defaults — no repos configured — and every surface names the path to create rather than failing; a file that exists but cannot be parsed is an error, since that machine was configured (ADR-0011).

Loaded by the CLI and the MCP server on every invocation that needs to discover repos (`search`, `count`, `read`, `repo`, `doctor`, `index`, and most `csl_*` MCP tools).

`csl config` prints that path, whether it loaded, and the settings in effect once defaults are applied; `csl config --help` carries an annotated example of every key below.

### Schema

| Field | Type | Required | Description |
|---|---|---|---|
| `dirs` | list of strings | yes | Root directories that contain your git checkouts. `csl` walks each recursively and records any directory whose child is `.git`. Tildes are expanded. Missing paths are skipped. |
| `index.hosts` | list of strings | no (default `[]`) | Allowlist of git hostnames. When non-empty, only repos whose remote origin hostname matches one of the listed values are indexed. An empty list means all discovered repos are included (no filtering). Repos with no remote are excluded whenever the list is non-empty. The hostname is the literal text of the remote URL, so an SSH alias counts as the host (see Discovery rules). |
| `hooks.post_merge.enabled` | bool | no (default `false`) | Master switch for the `csl hooks install` post-merge hook installer. See [hooks reference](hooks.md). |
| `hooks.post_merge.exclude` | list of strings | no (default `[]`) | Repos to skip when installing hooks. Each entry matches against the repo's absolute path or its `org/repo` name. Tildes are expanded. |
| `semantic.enabled` | bool | no (default `false`) | Whether the search daemon connects the embedder and loads the vector index at startup. When `false` the daemon serves lexical search only; the in-process `csl semantic` CLI, the `csl_semantic_search` MCP tool, and the web toggle still answer ad-hoc queries. See [semantic search](#semantic-search). |
| `semantic.sync` | bool | no (default `false`) | Whether `csl sync` also re-embeds the changed files of changed repos after the lexical reindex. When `false`, embeddings refresh only via `csl index --semantic`. |
| `semantic.ollama_url` | string | no (default `http://localhost:11434`) | Base URL of the Ollama server that serves the embedding model. |
| `semantic.embed_model` | string | no (default `unclemusclez/jina-embeddings-v2-base-code:f16`) | Ollama embedding model. Must be pulled (`ollama pull`). Changing it triggers a full re-embed on the next index run. |
| `semantic.dim` | int | no (default `768`) | Vector dimensionality of `embed_model`. Must match the model. |
| `sync.concurrency` | int | no (default `8`) | Parallel `git pull` workers for `csl sync`. `--concurrency` on the command line overrides it. |
| `daemon.idle_timeout_minutes` | int | no (default `10`) | How long the search daemon stays alive with no queries. Higher values keep the zoekt shards and semantic stores warm at the cost of resident memory. |
| `refresh.enabled` | bool | no (default `true`) | Whether `csl web` runs the periodic background refresh (pull + reindex changed repos). Manual refresh from the web UI works either way. |
| `refresh.interval_minutes` | int | no (default `15`) | How often the background refresh runs. Every cycle contacts every repo's remote, so keep it conservative. |
| `web.base_url` | string | no (derived from the port) | Where the csl web UI is reachable, used by `csl_show_file` to build the links it opens. Unset means `http://127.0.0.1:<port>`, where the port is `CSL_PORT` or 7424. Set to `http://csl.this` when the UI is fronted by d-man; an explicit value always wins. |
| `mcp.response_format` | string | no (default `text`) | Encoding every csl MCP tool answers in when a call does not pass its own `response_format` parameter; the parameter always wins. One of `text`, `json`, `jsonl` (JSON Lines), `toon` (Token-Oriented Object Notation), `csv`, `markdown-kv` (Markdown key-value pairs), `xml`. `text` is tool-specific (ripgrep-style for `csl_search`, key-value for most other tools); `json` is the structured object for clients that parse results. An unknown value fails every MCP tool call with an error naming this key. |

`layout`, `summary`, and `tmpdir` were carried over from csl's origin as a session launcher and have been removed — nothing read them. A file that still sets them loads unchanged, since unknown keys are ignored.

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
  # Hidden directories to index beyond the built-in .github and .claude.
  # Only git-tracked files under them count; .git is refused.
  allow_hidden_dirs:
    - .circleci

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
  embed_model: unclemusclez/jina-embeddings-v2-base-code:f16  # default; must be pulled in ollama
  dim: 768                                # must match embed_model

# `csl sync` pull parallelism.
sync:
  concurrency: 8

# Search daemon idle exit.
daemon:
  idle_timeout_minutes: 10

# Background index refresh inside `csl web` (see docs/web.md).
refresh:
  enabled: true
  interval_minutes: 15

# Where the web UI is reachable, for links `csl_show_file` opens.
web:
  base_url: http://127.0.0.1:7424

# Encoding the MCP tools answer in when a call passes no response_format of
# its own. One of text, json, jsonl, toon, csv, markdown-kv, xml.
mcp:
  response_format: text
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
- **MCP output shape is a default, not a lock.** `mcp.response_format` picks
  the encoding every `csl_*` tool answers in, and a call's own
  `response_format` parameter overrides it per call. A value outside the
  list above fails every MCP tool call with an error naming the key, so a
  typo can't silently fall back to `text`.

### Semantic search

Semantic (vector) search runs alongside the lexical zoekt index and is off by default. Embedding goes through a local [Ollama](https://ollama.com) server, so semantic features need Ollama running with the model pulled (`ollama pull unclemusclez/jina-embeddings-v2-base-code:f16`); lexical search has no Ollama dependency. Build the index with `csl index --semantic-all`.

Enabling it locally is a matter of corpus size: building the vector index scales with the number of repos, so as a rough guide it stays practical up to ~250 repos. Past that, keep the machine lexical-only or run the bulk build on a bigger machine — see [when local semantic search is worth it](semantic.md#when-local-semantic-search-is-worth-it).

- `semantic.enabled: true` lets the daemon connect the embedder and load the vector index at startup, so the `csl_semantic_search` MCP tool and the web toggle answer from a warm daemon.
- `semantic.sync: true` makes `csl sync` re-embed changed repos after the lexical reindex. The pass is incremental (only changed files) and best-effort: if Ollama or the model is unavailable, the lexical sync still succeeds. Leave it off to refresh embeddings manually with `csl index --semantic`.
- `semantic.ollama_url`, `semantic.embed_model`, and `semantic.dim` override the embedding backend per machine. Changing the model or its dimensionality drops the existing vector stores and re-embeds everything on the next index run. Interactive queries keep the model warm in Ollama for 20 minutes; bulk index runs unload it when they finish.

```yaml
semantic:
  enabled: true
  sync: false
  # ollama_url: http://localhost:11434   # default
  # embed_model: unclemusclez/jina-embeddings-v2-base-code:f16    # default; must be pulled in ollama
  # dim: 768                             # must match embed_model
```

### Discovery rules

- The walker uses 32 worker goroutines with a shared work channel. Each worker reads a directory; if any entry is named `.git`, the parent is recorded as a repo and the walker stops descending.
- Hidden directories (any starting with `.`) are skipped.
- Repo `name` is parsed from `.git/config` under `[remote "origin"]`. Both SSH (`git@host:org/repo.git`) and HTTPS (`https://host/org/repo.git`) forms are supported. When no remote is set, the name falls back to `<parent-dir>/<repo-dir>`.
- Repo `host` is extracted from the remote URL (`github.com`, `githost.example.com`) and surfaced in `csl repo --json` output and the `csl_repo_lookup` MCP tool. It is the literal text of the URL, so an SSH alias (`git@gh-work:org/repo.git`) yields `gh-work`, and that is the value `index.hosts` compares against.
- Repos dropped by `index.hosts` or `hooks.post_merge.exclude` are listed by `csl repo --list --skipped` with the reason for each (`--json` adds `remote` and `host`); `csl repo --list` shows only the survivors. When a filter drops every repo, the error names the filter and the counts, distinct from the "no git repos found under the dirs" that an empty `dirs` entry produces.

### Catalog descriptor

Discovery also reads a repo's identity from a catalog descriptor at its root, so `csl repo` and `csl_repo_lookup` can filter by `component`, `owner`, and `system` (`--component`/`--owner`/`--system` on the CLI). Precedence: `.csl-catalog.yaml` first, then `catalog-info.yaml` (Backstage's name), then `service-info.yaml` (the catalog service's name). The first Component entity in the file wins; `apiVersion` must be under `backstage.io` or `catalog.mad01` (any version, and a missing `apiVersion` is accepted). A repo with none of these files, or a descriptor with no `Component`, simply has no `component`/`owner`/`system` to match on. A descriptor that exists but fails to parse or follow is dropped silently from discovery, so it never breaks the walk; `csl doctor` (`catalog-descriptors`) is where that surfaces.

`.csl-catalog.yaml` is for a repo whose descriptor is not at the root, a monorepo or non-standard layout: one key, a repo-relative path that must stay inside the repo.

```yaml
descriptor: services/csl/service-info.yaml
```

A minimal descriptor at that path:

```yaml
apiVersion: catalog.mad01/v1alpha1
kind: Component
metadata:
  name: csl
spec:
  owner: platform
  system: thismoon
```

## State paths

State lives in the state directory: `$XDG_STATE_HOME/csl` when that variable holds an absolute path, otherwise `~/.local/state/csl`. The config file is not part of it.

**Installs made before the split keep their data in place.** When `~/.config/csl` exists on disk it stays the state directory, so upgrading a machine that has been running csl moves no files and re-indexes nothing. Fresh installs land in the state directory above. `csl docs` names the directory in effect.

All paths below are relative to that directory.

| Path | Purpose |
|---|---|
| `search-index/` | Zoekt index directory. Contains `*.zoekt` shard files and `state.json`. |
| `search-index/state.json` | Per-repo fingerprints used to decide which repos need re-indexing. |
| `search-index/.csl-sync.lock` | Lock file coordinating index writers across processes: a manual `csl sync` and the background refresh in `csl web` take it before pulling or indexing, and the ad-hoc builds (`csl search` reindex, the web fallback's first build) hold it or skip, so no two writers race each other on working trees, shards, or `state.json`. |
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

The fingerprint is `sha256(HEAD + "\n" + branch + "\n" + git status --porcelain + "\n" + index format version)`. Any change to committed state, branch, or working tree produces a new fingerprint, so `csl` knows to re-index that repo; so does a csl upgrade that changes what a shard holds, which re-indexes every repo once.

### Daemon lifecycle

- The daemon auto-starts when any command that needs it fails to reach the socket. It runs as a background process via `csl search --serve`, detached from the launching shell.
- Idle timeout: 10 minutes of no RPCs. Any call (including `Ping`) resets the timer.
- `SIGTERM` and `SIGINT` shut it down cleanly. The gRPC server graceful-stops, then the socket and PID file are removed.
- To start manually: `csl search --serve` (runs in foreground).
- To stop manually: `csl search --stop`.

See [architecture](architecture.md#search-daemon) for the full lifecycle.

## Environment

| Variable | Description |
|---|---|
| `CSL_CONFIG` | Path to the config file, overriding the default location. `--config` beats it. A leading `~` is expanded. |
| `CSL_PORT` | Port the web UI listens on (default 7424). `csl web` binds it, and every other surface assumes the UI is there when `web.base_url` is unset. `csl web --port` overrides it for that one process, but other processes cannot see the flag. |
| `XDG_CONFIG_HOME`, `XDG_STATE_HOME` | Roots of the config and state directories when set to an absolute path. Relative values are ignored, per the XDG spec. |
| `HOME` | Root of both directories when the XDG variables are unset. An unresolvable home is an error, never a path relative to the working directory. |
| `PATH` | The `EnsureDaemon` helper shells out to `csl search --serve` via `os.Executable()` rather than `PATH`, so daemon start works from any cwd. `git` is looked up on `PATH` for pull/fingerprint operations. |
| `EVENTS_BASE_URL` | Where the events service listens for best-effort telemetry events (default `http://127.0.0.1:7430`). Events are fire-and-forget; an unreachable events service never fails a csl operation. |

## Reset

To fully reset indexing state:

```sh
csl search --stop           # stop the daemon first
csl index --clean           # removes search-index/ from the state directory
csl search "anything"       # rebuilds the index on next search
```

`--clean` removes both the shards and `state.json`, so the next search re-indexes every configured repo. The config file is not touched, and neither is the semantic index — to reset that too, remove `semantic-index/` from the state directory and rebuild with `csl index --semantic-all` (or just rebuild: a model/dim/chunker change re-embeds automatically without the manual delete).
