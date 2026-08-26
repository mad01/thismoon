# csl configuration

## Where config lives

`csl` reads exactly one file: `~/.config/csl/config.yaml`. There is no
per-repo or per-directory override and no environment variable that moves
the path. The file is YAML, and every key in it is optional — a missing file
leaves `csl` on its defaults with no directories configured, and `csl config`
reports the file as "missing, defaults in use" rather than failing.

`csl config` prints the resolved path, whether the file loaded, and the
effective settings after defaults are applied. `csl config --help` carries
an annotated reference of every key, generated from the same source this
page documents.

Precedence is simple: config file values win when set, compiled-in defaults
fill every unset key, and a small number of CLI flags override specific
config values for a single invocation (`csl sync --concurrency`, `csl web
--port`). Flags never persist back to the file.

## Keys

- `dirs` (list of strings, default empty): root directories `csl` walks for
  git repos. Each entry is expanded for a leading `~`. The walker records
  every directory whose immediate child is `.git` and does not descend into
  nested repos once it finds one.
- `layout` (string, default `split`): inert, carried over from `csl`'s
  origin as a session launcher. Nothing in the current code reads it; it is
  printed by `csl config` so a config that sets it is not silently
  misreported. Accepted values are `split` and `tab`.
- `summary` (bool, default `false`): inert, same origin as `layout`. Only
  takes effect (were anything to read it) when `layout: tab` is also set.
- `tmpdir` (string, default empty): inert, same origin. Empty means the OS
  temp directory.

### `index`

- `index.hosts` (list of strings, default empty): allowlist of git remote
  hostnames. When non-empty, only repos whose origin remote matches one of
  the listed hosts are indexed; repos with no remote are excluded whenever
  the list is non-empty. An empty or omitted list indexes every discovered
  repo.

### `hooks.post_merge`

- `hooks.post_merge.enabled` (bool, default `false`): gates the deprecated
  `csl hooks install` post-merge hook installer. suspenders now owns git
  hooks and feeds `~/.config/csl/reindex.queue`, which `csl` still drains
  (`csl sync` / `csl index --drain`). Only `csl hooks install` reads this
  flag.
- `hooks.post_merge.exclude` (list of strings, default empty): repos to skip
  during indexing and `csl sync`, matched against the repo's absolute path
  or its `org/repo` name (exact match, no globs; `~` is expanded). This list
  is always live regardless of `enabled` — an excluded repo never enters
  either index.

### `sync`

- `sync.concurrency` (int, default `8`): parallel `git pull` workers for
  `csl sync`. Zero or negative falls back to 8. `csl sync --concurrency`
  overrides it for a single run.

### `semantic`

Controls the optional vector search path. It is independent of the lexical
zoekt index, which has no disable switch. Embedding goes through a local
[Ollama](https://ollama.com) server; `csl` bundles no model of its own.

- `semantic.enabled` (bool, default `false`): whether the search daemon
  loads the semantic index and connects the embedder at startup. When
  `false` the daemon serves lexical search only; the in-process
  `csl semantic` / `csl_semantic_search` paths still answer ad-hoc queries
  once the index exists.
- `semantic.sync` (bool, default `false`): whether `csl sync` also
  re-embeds the changed files of changed repos after the lexical reindex.
  Incremental and best-effort: a missing Ollama server or model never
  fails the sync.
- `semantic.ollama_url` (string, default `http://localhost:11434`): base
  URL of the Ollama server serving the embedding model.
- `semantic.embed_model` (string, default
  `unclemusclez/jina-embeddings-v2-base-code:f16`): the Ollama embedding
  model. Must be pulled (`ollama pull <model>`) before use. Changing it
  triggers a full re-embed of every store on the next index run.
- `semantic.dim` (int, default `768`): embedding dimensionality of
  `embed_model`. Must match the model; changing it (like changing the
  model) drops the existing vector stores and re-embeds everything on the
  next index run.

### `daemon`

- `daemon.idle_timeout_minutes` (int, default `10`): how long the
  background search daemon stays alive with no queries before exiting.
  Zero or negative falls back to 10. Higher values keep the zoekt shards
  and semantic stores mmap'd at the cost of resident memory.

### `refresh`

- `refresh.enabled` (bool, default `true`): whether `csl web` runs a
  periodic background refresh (pull + reindex changed repos), sharing the
  sync lock with a manual `csl sync`. Unset means enabled; manual refresh
  from the web UI's `/refresh` page works either way.
- `refresh.interval_minutes` (int, default `15`): how often the background
  refresh runs. Zero or negative falls back to 15. Every cycle contacts
  every repo's remote, so keep it conservative.

### `web`

- `web.base_url` (string, default `http://127.0.0.1:7424`): where the csl
  web UI is reachable. Used by `csl_show_file` to build the links it opens.
  Set to `http://csl.this` when the UI is fronted by d-man. `csl web --port`
  changes which port the process binds for a single run; it does not update
  this key.

## Environment variables

- `HOME`: root of the config and state paths (`~/.config/csl/...`).
- `PATH`: `git` is looked up on `PATH` for pull and fingerprint operations.
  The daemon-start helper shells out to the running binary via
  `os.Executable()` rather than `PATH`, so daemon start works from any
  working directory.
- `EVENTS_BASE_URL` (default `http://127.0.0.1:7430`): base URL of the
  local events service that `csl sync` and reindex runs archive activity
  to. The archive call is best-effort and synchronous with a short timeout;
  if the events service is down or the variable points nowhere reachable,
  the event is simply dropped and the command still succeeds. Never set
  during `go test` — test runs skip the call entirely.

There are no `CSL_*` configuration overrides.

## Example

```yaml
# ~/.config/csl/config.yaml

dirs:
  - ~/code/src/github.com
  - ~/workspace

index:
  hosts:
    - github.com
    - git.example.com

hooks:
  post_merge:
    exclude:
      - ~/workspace/large-monorepo
      - myorg/big-monorepo

sync:
  concurrency: 8

semantic:
  enabled: true
  sync: false
  ollama_url: http://localhost:11434
  embed_model: unclemusclez/jina-embeddings-v2-base-code:f16
  dim: 768

daemon:
  idle_timeout_minutes: 10

refresh:
  enabled: true
  interval_minutes: 15

web:
  base_url: http://127.0.0.1:7424
```
