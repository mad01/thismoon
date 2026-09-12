# csl

Local code search over your git checkouts. A CLI, a localhost web UI, and an MCP stdio server for Claude Code, all backed by [zoekt](https://github.com/sourcegraph/zoekt). For people who want fast cross-repo `grep`, and for agents that need to search, resolve repo paths, and read files without shelling out.

`csl` walks the directories you configure, indexes every git repo it finds, and searches them with zoekt's trigram index. You get `grep`-like queries across dozens of checkouts in milliseconds, without shipping your code to a cloud service. A short-lived search daemon spawns on demand from the same binary, keeps zoekt shards mmap'd across queries, and exits after ten minutes of idleness; there is no server to manage separately.

## How it works

The CLI, `csl mcp`, `csl web`, and the search daemon share the same internal search/index code. A query first tries the daemon over a Unix socket (auto-started if not running); if that fails, it falls back to opening the zoekt shards in-process. Index freshness is tracked per repo by a fingerprint of `HEAD` + branch + `git status`, so stale repos get reindexed in the background after results come back. `csl hybrid` / `csl_hybrid_search` add a second backend (vector embeddings over the same repos, searched by meaning) and fuse it with the lexical results by Reciprocal Rank Fusion.

## Install

Prebuilt darwin/arm64 tarballs ship on each [`csl/vX.Y.Z` release](https://github.com/mad01/thismoon/releases) with `checksums.txt` and a cosign keyless bundle; verification steps are in `docs/RELEASING.md`. With mise, which needs a tool alias because every thismoon component releases from this one repository:

```sh
mise tool-alias set csl github:mad01/thismoon
mise use -g "csl[version_prefix=csl/]"
```

With Homebrew, once the repository is public: `brew install mad01/tap/csl`. Both give you a binary with the tree-sitter grammars compiled in, so neither needs Go or a C toolchain.

Building from source needs the Go toolchain pinned in the repo's `go.mod` (1.26.2) and the Xcode Command Line Tools, because code chunking compiles tree-sitter grammars via cgo with no build-tag opt-out, which is also why `go install .../csl@latest` isn't a supported install path:

```sh
git clone https://github.com/mad01/thismoon.git
cd thismoon/services/csl
make install   # builds with version embedded, copies to ~/code/bin/csl, codesigns
```

`git` must be on `PATH` either way. Semantic search needs a running [Ollama](https://ollama.com) with the embedding model pulled (`ollama pull unclemusclez/jina-embeddings-v2-base-code:f16`); lexical search works without it.

Verify:

```sh
csl version
```

The walkthrough from install to configured index to MCP server to a supervised web UI is [docs/getting-started.md](docs/getting-started.md).

## Configuration

csl runs without a config file. `csl web`, `csl repo --list`, and `csl doctor` all work on a fresh machine and tell you which file to create. To index anything, create `config.yaml` in the config directory (`$XDG_CONFIG_HOME/csl`, or `~/.config/csl` when that variable is unset), listing the directories that contain your git checkouts (`--config` or `CSL_CONFIG` point csl at a different file, and `csl config` always prints the one in effect):

```yaml
dirs:
  - ~/code/src/github.com
  - ~/workspace

# Optional allowlist of git remote hosts. When set, repos with a
# non-matching host and repos with no remote are dropped from the index
# (`csl repo --list --skipped` shows which, and why), so leave it out
# until another search tool covers part of your checkouts. The host is the literal text of the remote URL:
# a checkout cloned as git@gh-work:org/repo.git has host gh-work, not the
# hostname your SSH config resolves it to.
# index:
#   hosts:
#     - github.com
#     - git.example.com

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

`csl` walks each directory concurrently and records every directory whose immediate child is `.git`; it doesn't descend into nested repos once it finds a `.git`. When `index.hosts` is set, only repos whose origin remote matches a listed host are indexed; use this to avoid indexing repos covered by another search tool. `csl repo --list` shows what survived discovery, `csl repo --list --skipped` shows what was dropped and why, and `csl repo --json` adds each repo's `remote` and `host`; the checklist for a repo that should be there and isn't is in [docs/getting-started.md](docs/getting-started.md#3-check-what-csl-discovered).

Discovery also reads a Backstage-shaped catalog descriptor from each repo's root (`catalog-info.yaml`, `service-info.yaml`, or a `.csl-catalog.yaml` pointer for a non-standard layout), so `csl repo` and `csl_repo_lookup` can find repos by who owns them, not just by name. `csl repo --json` adds `component`, `owner`, and `system` when a repo carries one; `--component`/`--owner`/`--system` filter on them. See [configuration](docs/configuration.md) for the file formats.

`hooks.post_merge.enabled` also gates the deprecated `csl hooks install` (see Usage); leave it unset unless you're deliberately using the legacy hook installer.

Moving the web UI off port 7424 for good takes `CSL_PORT` in the environment, or a `web.base_url` in the config file, not just `csl web --port`. The flag binds one process, while the MCP server builds `csl_show_file` links from a different one and can only see those two settings. Full reference: [config.md](config.md).

## Usage

```sh
csl repo --list                          # list every indexed repo
csl repo --list --skipped                # repos discovery dropped, with the reason
csl repo --list --system thismoon        # repos in the thismoon catalog system
csl repo --list --owner platform         # repos the platform team owns
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
csl web --port 7424     # serve the search UI on http://localhost:7424 (loopback only; csl.this with d-man)
```

A localhost web UI over the same index: a search box (lexical/semantic/hybrid) with example queries, an Examples tab with more, and results grouped by repo and file. Each hit links to the file on its git host and can be expanded inline. The web process is also the only long-lived part of csl: while it runs it pulls and reindexes every repo on a 15-minute loop (`refresh` in the config), serves the `/refresh` page, and backs the `csl_show_file` links the MCP server hands to the user. To keep it running, register it with a process manager such as t-man, giving the command as an absolute path: `t-man add --name csl-web -- "$HOME/.local/share/mise/shims/csl" web --port 7424` for a mise install. [docs/getting-started.md](docs/getting-started.md#8-optional-the-web-ui-as-a-background-service) explains the path choice.

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

`--scope user` makes the server available in every project; without it Claude Code registers csl for the current directory only. The registration stores the bare command name, so `csl` must be on the PATH of whatever launches Claude Code; a mise user who starts Claude Code from the Dock should register `"$HOME/.local/share/mise/shims/csl" mcp` instead. The ralph recipe does not register the server, because which agents run on a machine is machine-private wiring: run the command above once, or ship it from your own companion recipe (`docs/adr/0006` at the repo root, worked example under `examples/dotfiles/recipes/mcp-registration/`). Nothing needs to be running first, since the search daemon auto-starts on the first query and idles out on its own (see How it works). What does need to exist is `dirs` in the config file (see Configuration). Without at least one directory to walk, there's nothing to index. If a tool call comes back empty or erroring, run `csl doctor`.

The MCP server exposes fifteen `csl_*` tools:

- Repo: `csl_repo_lookup`, `csl_repo_info`, `csl_repo_health`, `csl_repo_pull`, `csl_repo_reindex`
- Search: `csl_search`, `csl_count`, `csl_query_validate`
- Semantic and hybrid: `csl_semantic_search`, `csl_hybrid_search`
- Read and info: `csl_read`, `csl_ls`, `csl_show_file`, `csl_index_info`
- Diagnosis: `csl_doctor`, the `csl doctor` checks as JSON, for a client with no shell

See [docs/mcp.md](docs/mcp.md) for the per-tool reference (inputs, return shape, defaults). Every tool takes a `response_format` parameter, `text` by default and `json` for the structured object; the other formats and the precedence rule are in the same document under Response formats.

To skip the per-call permission prompt, add `"mcp__csl__*"` to `permissions.allow` in `~/.claude/settings.json`. If [belt](../../tools/belt/README.md) is registered as well, its `prefer-csl` hint hands a multi-file `grep` or `find` inside an indexed repo back as the equivalent `csl_search` call, so the agent gets steered from both sides (`hints.prefer-csl.enabled`, on by default; details in `tools/belt/docs/hooks.md`).

### Add this to your CLAUDE.md

Registering the MCP server makes the tools available, but Claude will still reach for `find`, `ls`, `Glob`, or raw `grep` by default. Paste the snippet below into `~/.claude/CLAUDE.md` (user-level) or a project `CLAUDE.md` so Claude prefers `csl_*` tools for local repo work, or let the binary do it: `csl docs --claude-md >> ~/.claude/CLAUDE.md` prints the same text. It names all fifteen tools and keeps the agent on lexical search until you have built the semantic index. The binary embeds this block and a test holds the two copies byte-identical and checks every registered tool is named, so this is the copy to edit; the fleet example under `examples/dotfiles/CLAUDE.md.example` restates it.

```markdown
## Local Code Search (csl)

Use the `csl_*` MCP tools for repo discovery, code search, and file reads across local checkouts. Do not shell out to the `csl` CLI: the MCP tools and the CLI share one foundation, so if one is down the other is too.

Tools:
- Repo: `csl_repo_lookup`, `csl_repo_info`, `csl_repo_health`, `csl_repo_pull`, `csl_repo_reindex`
- Search: `csl_search`, `csl_count`, `csl_query_validate`
- Semantic and hybrid: `csl_semantic_search`, `csl_hybrid_search` (only once semantic search is set up, see Search)
- Read and info: `csl_read`, `csl_ls`, `csl_show_file`, `csl_index_info`
- Diagnosis: `csl_doctor`

### Search
- Default to `csl_search`. Lexical search always works and needs nothing running.
- `csl_semantic_search` and `csl_hybrid_search` need the semantic index built (`csl index --semantic-all`) and Ollama running. Until then semantic answers `available=false` and hybrid degrades to lexical-only. Reach for them on "where do we handle X" questions only when `csl_index_info` reports `semantic.built: true`; otherwise stay lexical.
- Use `csl_read` for file contents you need yourself and `csl_show_file` to put a file section in front of the user (it needs `csl web` running).
- Tools return `text` by default. Set `mcp.response_format` in `config.yaml` to change the default, and pass `response_format: "json"` on a call when you need the structured object.
- If a tool errors or comes back unexpectedly empty, call `csl_doctor` before retrying.
- If `csl_repo_lookup` finds no match for the repo you are working in, it sits outside csl's configured `dirs`: use `grep`, `find`, or `Glob` there instead. Plain `grep` is also fine for piping and filtering command output.

### Repo discovery
- Use `csl_repo_lookup` or `csl_repo_info` to find repos. Do not use `find`, `ls`, `Glob`, or shell to manually search for repo directories.
- `csl_repo_info` returns git health (branch, dirty files, index staleness, suggested action). Call it before starting work on a repo to decide whether to commit, stash, pull, or reindex.
- `csl_repo_lookup` returns `remote` and `host` fields; use them to branch behavior per git host when needed. It also returns `component`, `owner`, and `system` when a repo carries a catalog descriptor, and takes `component`/`owner`/`system` filters (case-insensitive regex, same rules as `name`) for "which repos does team X own" or "which repos are in system Y" questions; a repo with no descriptor never matches those filters.
- If lookup returns empty `matches` and a non-empty `dropped`, csl found the checkout but a config filter (`index.hosts` or the exclude list) removed it; report the `reason` to the user. Empty both means the repo is not checked out locally or not under csl's configured dirs; say so, don't guess paths.
- Use `csl_repo_pull` before creating branches on repos that may be behind (it has safety checks for dirty state).
- Use `csl_repo_reindex` after significant changes so `csl_search` results stay current.
- Use `csl_repo_health` for a fleet-wide sweep of uncommitted or unpushed work, for example before switching machines.
- Exploring needs no pull. Before changing a repo, or telling the user something about its current state they will act on, run `csl_repo_info` first. On a dirty tree never stash or discard on your own; on a stale index pull only when the tree is clean.

### Query syntax

zoekt queries look like grep but have important differences:

- AND is strict: space-separated terms must ALL appear in the SAME file. Use 1-2 terms and narrow with filters, not 3+ chained terms.
- OR: use `|` with no spaces (`foo|bar`) or lowercase `or`. Uppercase `OR` is a literal string. Spaces around `|` break it.
- Filters: `repo:name` (not `r:`), `f:\.go$` (not `file:`), `lang:go`, `-term` (NOT).
- File filters are regex, not glob: `f:.*\.go$` not `f:*.go`.
- Exact phrase: `"foo bar"` requires that exact string on one line. For proximity, use regex: `foo.*bar`.
- Validate: call `csl_query_validate` to see how zoekt parsed your query. This is especially useful when you get zero results.

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

`config.yaml` lives in the config directory (see Configuration). Everything csl writes lives in the state directory (`$XDG_STATE_HOME/csl`, or `~/.local/state/csl` when that variable is unset):

- `search-index/`: lexical index (`state.json` per-repo fingerprint/branch/dirty/indexed-at, plus `<shard-hash>.zoekt` shard files).
- `semantic-index/`: per-repo vector stores (embeddings come from Ollama; no model files live here).
- `search-daemon.sock`, `search-daemon.pid`, `search-daemon.log`: the search daemon's socket, PID file, and rotated log.
- `reindex.queue`: repo paths queued by the suspenders `csl-reindex` post-merge hook; drained by `csl sync` / `csl index --drain`.

An install made before config and state were split keeps using `~/.config/csl` for both: when that directory exists it stays the state directory, so upgrading moves nothing and re-indexes nothing.

`make install` puts the binary at `~/code/bin/csl`; `make install PREFIX=/somewhere/else` puts it there instead.

## Develop

```sh
make build    # ./csl binary (fetches native ONNX/tokenizer libs first, macOS arm64)
make test     # go test -timeout 120s ./... (unit tests use a fake embedder, no native libs needed)
make lint     # golangci-lint run ./...
```

Further docs:

- [Getting started](docs/getting-started.md): install with mise, configure, check discovery, first search, MCP server, web UI under t-man.
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
