# Getting started

This walks from a blank machine to your first cross-repo search. Allow about five minutes.

## Prerequisites

- Go 1.26 or newer. Check with `go version`.
- Optional, for semantic search only: [Ollama](https://ollama.com) (step 9).
- `git` on `PATH`. `csl` reads `.git/config` directly for repo names, but uses `git` for pull and fingerprinting.
- At least one local git checkout.

## 1. Install

csl builds from a checkout only: code chunking compiles tree-sitter grammars via cgo, so `go install .../csl@latest` isn't a supported install path.

```sh
git clone https://github.com/mad01/thismoon.git
cd thismoon/services/csl
make install
```

`make install` builds with the build metadata embedded (short git hash, full commit, release tag, build time), copies the binary to `~/code/bin/csl`, strips the macOS quarantine attribute, and ad-hoc codesigns it.

Verify:

```sh
csl version
```

## 2. Configure

Create `~/.config/csl/config.yaml` and list the directories that contain your git checkouts. One entry per root. `csl` walks each concurrently.

```yaml
dirs:
  - ~/code/src/github.com
  - ~/code/src/github.com/myorg
  - ~/workspace
```

Tildes are expanded. Missing directories are skipped, not errored.

## 3. List discovered repos

```sh
csl repo --list
```

Output, one line per repo, tab-separated name and path:

```
myorg/service-a	/Users/you/code/src/github.com/myorg/service-a
myorg/service-b	/Users/you/code/src/github.com/myorg/service-b
mad01/thismoon	/Users/you/code/src/github.com/mad01/thismoon
```

Repo names come from the `origin` remote URL in `.git/config`. If a checkout has no remote, the name falls back to the parent directory plus the repo directory.

If the list is empty, the `dirs` paths don't contain git repos. Re-check the config; use absolute paths to rule out tilde expansion issues.

`csl repo` without `--list` opens an interactive fuzzy picker and prints the chosen repo's path; `csl repo <query>` skips the picker and prints the single match (erroring if the query is ambiguous). That makes a one-line jump function:

```sh
# ~/.zshrc
repo() { local d=$(csl repo "$@"); [[ -n "$d" ]] && cd "$d"; }
```

Now `repo thismoon` drops you into the checkout, and bare `repo` gives you the picker.

## 4. Run your first search

```sh
csl search "func Walk"
```

On the first call with no prior index, `csl` prints progress:

```
Indexing 17 repo(s)...
  [1/17] myorg/service-a
  [2/17] myorg/service-b
  ...
myorg/service-a/internal/walker/walker.go
mad01/thismoon/services/csl/internal/repo/finder/walker.go
```

The default output mode, `files_with_matches`, prints one line per matched file as `<repo>/<path>`. To see the matching lines instead:

```sh
csl search "func Walk" --output-mode content --context-lines 2
```

## 5. Filter a search

```sh
csl search "TODO" --repo thismoon --lang go
```

All filters compose. Under the hood, each flag adds a zoekt clause to the query: `repo:<regex>`, `lang:<name>`, `file:<regex>`, `case:yes`.

## 6. Check index health

```sh
csl doctor
```

Output:

```
Index directory: /Users/you/.config/csl/search-index
Index size:      84.3 MB
Total repos:     17
Index shards:    17 (17 healthy, 0 corrupted)
Search daemon:   running

Healthy (17):
  myorg/service-a                          indexed 3m12s ago
  ...
```

`doctor` reports corrupted shards, stale repos, dirty working trees, and daemon status in one view. Run it when results look off.

## 7. Keep the index fresh

`csl search` re-indexes stale repos in the background after answering, so day-to-day searches keep themselves current. To also pull every repo and reindex whatever changed upstream in one pass:

```sh
csl sync
```

`csl sync` pulls each repo fast-forward-only (skipping dirty trees, non-default branches, and detached HEADs), then batch-reindexes the changed ones. Safe to run any time.

If you like it as a habit, wrap it in a shell function:

```sh
# ~/.zshrc
repo-sync() { csl sync; }
```

Machines provisioned through the ralph recipe in this repo (`recipes/csl`) get the `repo-sync` and `repo` functions defined automatically.

## 8. Register the MCP server

```sh
claude mcp add --scope user csl -- csl mcp
claude mcp list
```

Expected output:

```
csl: csl mcp - ✓ Connected
```

From a Claude Code session, the agent can now call `csl_search`, `csl_repo_lookup`, `csl_read`, and the nine other `csl_*` tools directly. See the [MCP reference](mcp.md) for per-tool contracts.

## 9. Optional: semantic search

Everything so far is lexical: exact text matching with no extra dependencies. Semantic search (match by meaning: "retry failed requests" finds the backoff loop whatever it's named) needs a local [Ollama](https://ollama.com) server with an embedding model pulled:

```sh
brew install ollama
brew services start ollama
ollama pull unclemusclez/jina-embeddings-v2-base-code:f16
```

Enable it and build the vector index:

```yaml
semantic:
  enabled: true
```

```sh
csl index --semantic-all
csl semantic "walk directories looking for git repos"
```

The first build embeds every repo and takes a while; later runs only re-embed changed files. `csl hybrid <query>` fuses both backends. See [semantic search](semantic.md) for how it works and how to pick a different model.

## Next

- [CLI reference](cli.md) — every subcommand and flag.
- [Configuration reference](configuration.md) — every file under `~/.config/csl/`.
- [Semantic search](semantic.md) — lexical vs semantic, the vector index, embedding models.
- [Architecture](architecture.md) — how the daemon, index, and MCP server fit together.
