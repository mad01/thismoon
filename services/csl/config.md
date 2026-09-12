# csl configuration

## Where config lives

`csl` reads exactly one file per run, resolved in this order:

1. `--config <path>`, a persistent flag on every subcommand.
2. `$CSL_CONFIG`.
3. `config.yaml` in the XDG config directory: `$XDG_CONFIG_HOME/csl` when
   that variable holds an absolute path, otherwise `~/.config/csl`.

A leading `~` is expanded in the flag and the environment variable, so both
work under launchd, where no shell expands them first. There is no per-repo
or per-directory override.

**The file is optional and so is every key in it.** A machine with no config
file runs on defaults: `csl web` serves an empty UI, `csl repo --list` prints
an empty list, `csl doctor` passes, and each of them names the file to create.
Nothing has to exist before csl starts. What is not tolerated is a file that
exists and cannot be parsed — that machine was configured, and running on
defaults there would hide the mistake, so those commands fail naming the file
(ADR-0011).

The same distinction runs through the empty-result messages: with no config
file you are told which file to create, while a config that is present and
still discovers no repos is reported as an error, because a configured
machine finding nothing is a misconfiguration rather than a starting state.

`csl config` prints the resolved path, whether the file loaded, and the
effective settings after defaults are applied. `csl config --help` carries
an annotated reference of every key, generated from the same source this
page documents.

Precedence is simple: config file values win when set, compiled-in defaults
fill every unset key, and a small number of CLI flags override specific
config values for a single invocation (`csl sync --concurrency`, `csl web
--port`). Flags never persist back to the file.

## Where state lives

The config file is the only thing under the config directory. Everything
`csl` writes — the lexical index (`search-index/`), the vector stores
(`semantic-index/`), the search server's socket, PID file and log, and
`reindex.queue` — lives in the state directory: `$XDG_STATE_HOME/csl`, or
`~/.local/state/csl` when that variable is unset.

Installs made before the split are the exception, and deliberately so: when
`~/.config/csl` holds a `search-index/` directory it stays the state
directory, so an upgrade moves no files and re-indexes nothing. `csl doctor`
and the `operating.md` doc (`csl docs`) both name the directory in effect.

The index is what settles it, not the directory. The fleet recipe symlinks
`config.yaml` into `~/.config/csl` on every machine, so the directory exists
even where csl has never indexed anything; treating that as evidence would
leave every provisioned machine on the pre-split path forever. A machine with
the config symlink and no index lands in the state directory like any fresh
install.

## Keys

- `dirs` (list of strings, default empty): root directories `csl` walks for
  git repos. Each entry is expanded for a leading `~`. The walker records
  every directory whose immediate child is `.git` and does not descend into
  nested repos once it finds one.

`layout`, `summary`, and `tmpdir` were carried over from `csl`'s origin as a
session launcher and have been removed: nothing read them. A config file
that still sets them loads unchanged — unknown keys are ignored — but
`csl config` no longer echoes them back.

### `index`

- `index.hosts` (list of strings, default empty): allowlist of git remote
  hostnames. When non-empty, only repos whose origin remote matches one of
  the listed hosts are indexed; repos with no remote are excluded whenever
  the list is non-empty. An empty or omitted list indexes every discovered
  repo. The host is the literal text between `@` and `:` (SSH) or after
  `://` (HTTPS) in the remote URL, so a checkout cloned through an SSH alias
  such as `git@gh-work:org/repo.git` has host `gh-work` and must be listed
  by that name. `csl repo --list` shows only the survivors;
  `csl repo --list --skipped` lists the dropped repos with the reason for
  each, and the repos-discovered check in `csl doctor` carries the counts. A
  list that drops every repo fails with a message naming the filter, distinct
  from the "no git repos found" a `dirs` entry that holds none produces.

### `hooks.post_merge`

- `hooks.post_merge.enabled` (bool, default `false`): gates the deprecated
  `csl hooks install` post-merge hook installer. suspenders now owns git
  hooks and feeds `reindex.queue` in the state directory, which `csl` still
  drains (`csl sync` / `csl index --drain`). Only `csl hooks install` reads
  this flag; the hook it writes has the resolved queue path baked in, since
  a git hook cannot ask `csl` where the queue is.
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

- `web.base_url` (string, default derived from the port): where the csl web
  UI is reachable. Used by `csl_show_file` to build the links it opens. When
  unset it is `http://127.0.0.1:<port>`, where the port is `CSL_PORT` or
  7424 — so exporting `CSL_PORT` moves the UI and its links together. Set
  this key to `http://csl.this` when the UI is fronted by d-man; an explicit
  value always wins.

  `csl web --port` binds a different port for that one process. Inside it
  the links follow the flag, but other processes (the MCP server, `csl
  doctor` in another shell) only see `CSL_PORT` and this key, so pin one of
  those when the UI permanently moves.

### `mcp`

- `mcp.response_format` (string, default `text`): the encoding every csl MCP
  tool answers in when a tool call does not pass its own `response_format`
  parameter. The parameter always wins over this key. Valid values: `text`,
  `json`, `jsonl` (JSON Lines), `toon` (Token-Oriented Object Notation),
  `csv`, `markdown-kv` (Markdown key-value pairs), and `xml`. `text` is
  tool-specific: ripgrep-style lines for `csl_search`, key-value output for
  most other tools. `json` is the structured object the tools returned before
  this key existed, for clients that parse results. Any other value makes
  every MCP tool call fail with an error naming `mcp.response_format`. That
  is deliberate: a typo in the file should be loud, not silently ignored.

## Environment variables

- `CSL_CONFIG`: path to the config file, overriding the default location.
  `--config` beats it. A leading `~` is expanded.
- `CSL_PORT` (default `7424`): the port the web UI listens on. `csl web`
  binds it (`--port` overrides for that process), and every other surface
  assumes the UI is there when `web.base_url` is unset.
- `XDG_CONFIG_HOME` / `XDG_STATE_HOME`: roots of the config and state
  directories when set to an absolute path. Relative values are ignored,
  per the XDG spec.
- `HOME`: root of both directories when the XDG variables are unset. An
  unresolvable home is an error, not a path relative to the working
  directory.
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

## Example

```yaml
# $XDG_CONFIG_HOME/csl/config.yaml (or ~/.config/csl/config.yaml)

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

mcp:
  response_format: text
```
