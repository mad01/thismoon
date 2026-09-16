# CLI reference

Every `csl` subcommand, grouped by purpose. Flags use long form unless a short flag exists. Required flags are marked in the table.

Every command that reads repos loads the config file — `--config`, else `$CSL_CONFIG`, else `config.yaml` in the XDG config directory — and walks the configured `dirs`. See [configuration](configuration.md) for the schema.

## Index summary

| Command | Purpose |
|---|---|
| [`csl search`](#csl-search) | Search code across all indexed repos |
| [`csl count`](#csl-count) | Count matches, optionally grouped by repo or language |
| [`csl hybrid`](#csl-hybrid) | Fuse lexical and semantic results with Reciprocal Rank Fusion |
| [`csl read`](#csl-read) | Read a file from a named repo by relative path |
| [`csl repo`](#csl-repo) | List or interactively pick a repo |
| [`csl index`](#csl-index) | Manage the zoekt index (status, repair, clean) |
| [`csl hooks`](#csl-hooks) | Install a `post-merge` git hook to auto-reindex on `git pull` |
| [`csl doctor`](#csl-doctor) | Report index and daemon health |
| [`csl config`](#csl-config) | Show which config file is read and the settings in effect |
| [`csl query`](#csl-query) | Validate a zoekt query without running it |
| [`csl mcp`](#csl-mcp) | Start the MCP stdio server (for Claude Code) |
| [`csl docs`](#csl-docs) | Print the operating doc, or the CLAUDE.md section with `--claude-md` |
| [`csl shell-init`](#csl-shell-init) | Print the `repo` and `repo-sync` shell functions for `eval` |
| [`csl version`](#csl-version) | Print the build version |

---

## `csl search`

Search code across all indexed repos using zoekt query syntax.

### Synopsis

```sh
csl search [flags] <pattern>
csl search --serve                 # start daemon in foreground
csl search --stop                  # stop the running daemon
```

### Description

`csl search` tries the daemon first, falling back to an in-process search if the socket is unreachable. On the first search after a clean install (or after `csl index --clean`), `csl` indexes every configured repo synchronously and reports progress on stderr, then runs the query.

After every search, stale repos (new commits, dirty tree, branch switched) are re-indexed in the background before the next call.

### Flags

| Flag | Default | Description |
|---|---|---|
| `-r, --repo <regex>` | `""` | Restrict to repos matching this regex or substring |
| `-l, --lang <name>` | `""` | Restrict to a single language (e.g. `go`, `python`, `swift`) |
| `-f, --file <regex>` | `""` | Restrict to file paths matching this regex (e.g. `\.go$`) |
| `-o, --output-mode <mode>` | `files_with_matches` | `files_with_matches` prints unique paths; `content` prints matching lines with context, grouped per file (see below) |
| `-C, --context-lines <n>` | `0` | Context lines around each match. Only applies to `content` mode |
| `--limit <n>` | `50` | Maximum number of file results |
| `--case-sensitive` | `false` | Force case-sensitive matching. Default is smart case (case-insensitive unless the query has uppercase) |
| `--reindex` | `false` | Synchronously re-index before searching |
| `--json` | `false` | Emit results as a JSON array of match objects |
| `--toon` | `false` | Emit results as [TOON](https://github.com/alpkeskin/gotoon), a compact format designed for LLM consumption |
| `--serve` | `false` | Run the search daemon in the foreground (blocks until `SIGINT` or idle timeout) |
| `--stop` | `false` | Send a shutdown request to the running daemon |

### Query syntax

`csl` passes the pattern to zoekt's query parser. The common operators:

| Syntax | Meaning |
|---|---|
| `foo` | Literal substring match |
| `foo.*bar` | Regular expression |
| `"func Walk"` | Quoted phrase — exact match with spaces |
| `foo bar` | Space-separated terms are AND |
| `foo\|bar` | OR |
| `-test` | NOT |
| `repo:<regex>` | Restrict to repos matching |
| `file:\.go$` | Restrict to file paths matching |
| `lang:go` | Restrict to a language |
| `sym:Name` | Restrict to symbol definitions (functions, types, methods, classes); content hits carry `kind` |
| `case:yes` | Case-sensitive this term |

`content` mode prints ripgrep `--heading` style, the same grammar as the `csl_search` MCP tool's text format: one `repo/path` header per file, `LINE:COL:text` for a matching line (`COL` is the 1-based column where the match starts, as with ripgrep `--column`; the MCP text form prints `LINE:text` without it), `LINE-text` for a context line, and a blank line between files. Hits that sit within each other's context window merge into one block, so every line prints once; `--` separates blocks that are not adjacent in the file. A `sym:` hit's line ends with `kind=<kind>` (plus `parent=<name>` when nested). `--json` keeps one entry per matching line, each with its own `before`/`after` context.

Regex metacharacters inside the pattern need shell-escaping too. Validate a tricky query first with [`csl query`](#csl-query).

### Examples

**Find a function across every repo:**

```sh
csl search "func Walk"
```

**Narrow to one repo and one language, show surrounding context:**

```sh
csl search "fmt\.Errorf" --repo thismoon --lang go --output-mode content -C 3
```

**Machine-readable output for piping into tools:**

```sh
csl search "import.*cobra" --file "\.go$" --limit 10 --json
```

**Force a re-index before searching, useful after a manual git checkout:**

```sh
csl search --reindex "NewBuilder"
```

---

## `csl count`

Count matches across indexed repos, optionally grouped.

### Synopsis

```sh
csl count [flags] <pattern>
```

### Description

Takes the same query syntax as `csl search`. Always goes through the daemon first, falling back to in-process. Exits with a single total unless `--group-by` is set.

### Flags

| Flag | Default | Description |
|---|---|---|
| `-r, --repo <regex>` | `""` | Restrict to repos matching |
| `-l, --lang <name>` | `""` | Restrict to a language |
| `--group-by <key>` | `""` | Group counts by `repo` or `language` |
| `--json` | `false` | Emit results as JSON |

### Examples

**Single total:**

```sh
csl count "TODO"
```

**Per-repo breakdown:**

```sh
csl count "func.*Error" --group-by repo
```

Output:

```
mad01/thismoon                           87
myorg/service-a                          55

total: 142
```

**Per-language breakdown, filtered to Go files:**

```sh
csl count "import" --lang go --group-by language
```

---

## `csl hybrid`

Search code by fusing lexical (zoekt) and semantic (vector) results using Reciprocal Rank Fusion (RRF).

### Synopsis

```sh
csl hybrid [flags] <query>
```

### Description

`csl hybrid` runs both a zoekt lexical search and a semantic vector search, then merges the results with Reciprocal Rank Fusion (RRF). A file's fused score is the sum of `1/(k+rank)` over the result lists it appears in, so a file present in both lists outranks a file topping only one. Fusion is file-level, with lexical results collapsed to one rank per `(repo, path)`. Default k=60 (Cormack et al. 2009).

Daemon-first with in-process fallback, same as `csl search`. When the semantic index is not built, degrades to lexical-only and prints a note to stderr.

### Flags

| Flag | Default | Description |
|---|---|---|
| `-r, --repo <regex>` | `""` | Restrict to repos matching this regex or substring |
| `-l, --lang <name>` | `""` | Restrict to a single language (e.g. `go`, `python`) |
| `--limit <n>` | `50` | Maximum number of results |
| `--rrf-k <n>` | `60` | RRF constant k |
| `--expand <n>` | `0` | Extra source lines above and below each matched chunk |
| `--json` | `false` | Emit results as JSON |

### Examples

**Natural-language query with exact-match coverage:**

```sh
csl hybrid "retry failed HTTP requests"
```

**Narrow to one repo and language:**

```sh
csl hybrid "parse config file" --repo thismoon --lang go
```

**JSON output:**

```sh
csl hybrid "error handling middleware" --limit 10 --json
```

---

## `csl read`

Read a file from a named local repo.

### Synopsis

```sh
csl read --repo <name> [flags] <path>
```

### Description

Resolves `--repo` against every configured repo using substring match (not regex). The file path is relative to the repo root. Lines are 1-based; `--start-line 0` or `--end-line 0` means open-ended.

### Flags

| Flag | Required | Default | Description |
|---|---|---|---|
| `-r, --repo <name>` | yes | — | Repo name to read from. Substring match |
| `--start-line <n>` | no | `0` | First line to show (1-based, 0 for start) |
| `--end-line <n>` | no | `0` | Last line to show (1-based inclusive, 0 for end) |
| `--json` | no | `false` | Emit as JSON: `{repo, path, lines: [{number, text}]}` |

### Examples

```sh
csl read internal/cli/root.go --repo thismoon
csl read main.go --repo thismoon --start-line 10 --end-line 30
csl read README.md --repo thismoon --json
```

---

## `csl repo`

Discover and pick git repos.

### Synopsis

```sh
csl repo                    # interactive picker
csl repo <query>            # print the single matching repo's path
csl repo --list             # tab-separated name + path
csl repo --list --skipped   # the repos discovery dropped, with the reason
csl repo --json             # structured output
csl repo --toon             # TOON-encoded output for LLMs
csl repo --component <s>    # only repos whose catalog descriptor names this component
csl repo --owner <s>        # only repos whose catalog descriptor has this owner
csl repo --system <s>       # only repos whose catalog descriptor is in this system
```

### Description

Without flags, `csl repo` opens an interactive picker and prints the absolute path of the selected repo on stdout, useful for `cd $(csl repo)` workflows.

With a query argument, it skips the picker and prints the path of the single repo whose `org/repo` name contains the query (case-insensitive). Zero or multiple matches exit non-zero; the multi-match error lists the candidates. Combined with `--list`/`--json`/`--toon`, a query filters the output instead of erroring.

`--component`, `--owner`, and `--system` narrow the set to repos whose root catalog descriptor (`catalog-info.yaml` or `service-info.yaml`, a Backstage-shaped Component; see [configuration](configuration.md#catalog-descriptor)) has a matching `metadata.name`, `spec.owner`, or `spec.system` (case-insensitive substring; a repo with no descriptor never matches, whatever the pattern). They compose with the positional query and with `--list`/`--json`/`--toon` (all set filters must match). On their own, without a positional query, they open the picker over the narrowed set; when exactly one repo is left they print its path directly instead. A query that matches nothing (positional or catalog filters) errors with `no repos match query "<set fields>"`, naming the fields that were set.

`--skipped` answers the opposite question: which repos the walk found under `dirs` and then dropped, and why. Each line is `name<TAB>path<TAB>reason`, where the reason is one of `host <h> not in index.hosts`, `no remote (index.hosts is set)`, or `excluded by hooks.post_merge.exclude`. It implies `--list`, composes with `--json`/`--toon` (each entry then also carries `remote` and `host`, which is where an SSH alias shows up as the host), and a query filters it the same way. When a filter drops every repo, the plain commands fail with a message that names the filter and the counts and points here.

The picker is csl's own (`internal/picker`), not [go-fuzzyfinder](https://github.com/ktr0731/go-fuzzyfinder): it uses go-fuzzyfinder's matching package so the fuzzy match behaves the same, but draws with `tcell` directly because it needs to color parts of a line, which go-fuzzyfinder can't do. Each line reads `org/repo  <component>  owner:<owner>  system:<system> @ <path>`: the component name appears only when it differs from the repo's short name, the owner and system labels are dim, and matched characters highlight green. Colors: repo name default, component cyan, owner magenta, system blue, path and labels dim. `NO_COLOR` (any non-empty value) turns all color off. The query works like fzf's extended search: space-separated words are matched on their own and every one has to hit, in any order, so `system:foo bar` finds the lines that hold both. The selected row sits on a band derived from the terminal's own background: before taking the screen the picker asks for that color over OSC 11 and steps it a little toward white on a dark theme or toward black on a light one, so the band reads as the theme's selection tone. A terminal that does not answer within 100ms gets the palette's bright black instead. Dim text on that row is lifted to the default foreground so the path stays readable. Keys: Enter selects, Esc or Ctrl-C aborts, Up/Down or Ctrl-P/Ctrl-N (also Ctrl-K/Ctrl-J) move the cursor. The query line edits like a shell prompt: Left/Right or Ctrl-B/Ctrl-F move by character, Alt-B/Alt-F and Alt-Left/Alt-Right by word, Home/End or Ctrl-A/Ctrl-E jump to either end, Backspace and Delete (or Ctrl-D) remove the character on either side of the cursor, Ctrl-W removes the word before it, and Ctrl-U removes everything before it.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--list` | `false` | Print every repo, tab-separated: `name<TAB>path` |
| `--skipped` | `false` | Print the repos discovery dropped instead, tab-separated: `name<TAB>path<TAB>reason` (implies `--list`) |
| `--json` | `false` | Emit array of `{name, path, remote, host, component?, owner?, system?}` (implies `--list`; with `--skipped`, each entry adds `reason`) |
| `--toon` | `false` | TOON-encoded output under a `repos` key (implies `--list`; `skipped` key with `--skipped`) |
| `--component <s>` | `""` | Only repos whose catalog descriptor names this component (`metadata.name`), case-insensitive substring |
| `--owner <s>` | `""` | Only repos whose catalog descriptor has this owner (`spec.owner`), case-insensitive substring |
| `--system <s>` | `""` | Only repos whose catalog descriptor is in this system (`spec.system`), case-insensitive substring |

### Examples

```sh
cd $(csl repo)                      # pick and cd
cd $(csl repo thismoon)             # jump straight to the match
csl repo --list | grep service-
csl repo --json | jq '.[] | .host' | sort -u
csl repo --list --skipped           # why is my repo missing?
csl repo --json --skipped | jq '.[] | select(.reason | startswith("host"))'
csl repo --system thismoon          # open the picker over one system's repos
csl repo --list --owner platform    # everything the platform team owns
csl repo --json --system thismoon | jq '.[] | .name'
```

A shell function makes the jump a habit — bare `repo` opens the picker, `repo <query>` cd's straight there. [`csl shell-init`](#csl-shell-init) prints it, so the body has one owner:

```sh
# ~/.zshrc
eval "$(csl shell-init zsh)"
```

---

## `csl index`

Manage the zoekt index.

### Synopsis

```sh
csl index                   # re-index stale repos only
csl index --all             # force re-index of every repo
csl index --repo <path>     # re-index a single repo by absolute path
csl index --status          # report per-repo freshness
csl index --repair          # validate and drop corrupted shards
csl index --clean           # delete the entire index directory
```

### Description

By default, `csl index` diffs the current repo fingerprints against `state.json` and re-indexes only the repos whose fingerprint changed. The fingerprint covers HEAD, branch, `git status --porcelain`, and the index format version, so any commit, checkout, or working-tree change triggers a re-index, and so does an upgrade that changes what a shard holds (every repo re-indexes once).

`--repair` opens every `*.zoekt` shard, parses its metadata, and removes any that fail to read. Run `csl index` after repair to rebuild affected repos.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--all` | `false` | Re-index every repo, regardless of fingerprint |
| `--repo <path>` | `""` | Re-index a single repo by absolute path. Skips the global staleness check; updates that repo's `state.json` entry. Used by the `post-merge` git hook |
| `--status` | `false` | Print a status table instead of indexing |
| `--clean` | `false` | Remove `search-index/` from the state directory entirely |
| `--repair` | `false` | Validate shards, remove corrupted ones |
| `--json` | `false` | JSON output for `--status` and `--repair` |

### Examples

**Status table:**

```sh
csl index --status
```

Output:

```
REPO                                     STATUS     DIRTY    INDEXED AT           BRANCH
mad01/thismoon                           fresh      no       2m12s ago            main
myorg/service-a                          stale      yes      1h4m ago             feat/auth
```

**Full rebuild:**

```sh
csl index --all
```

**Repair after a crash or bad disk:**

```sh
csl index --repair
csl index
```

**Reindex a single repo (what the post-merge hook calls):**

```sh
csl index --repo ~/code/src/github.com/myorg/foo
```

---

## `csl hooks`

Install a `post-merge` git hook into every discovered repo so the index refreshes automatically after `git pull`.

### Synopsis

```sh
csl hooks install               # write/update the hook in every non-excluded repo
csl hooks install --dry-run     # preview without writing
csl hooks uninstall             # remove csl-managed hooks
csl hooks status                # report per-repo hook state
```

### Description

Off by default. Enable via `hooks.post_merge.enabled: true` in the config file, then run `csl hooks install`. The installer writes the same hook script into every repo discovered through `dirs`, applying `hooks.post_merge.exclude`. Re-running is idempotent — identical files are skipped — so it is safe to wire into `ralph apply`.

The hook calls `csl index --repo <path>` in the background after each pull, keeping the search index in sync without blocking the pull.

See [hooks reference](hooks.md) for the config schema, exclusion semantics, the hook script itself, and the full subcommand reference.

---

## `csl doctor`

Run csl's self-checks, one line per check.

### Synopsis

```sh
csl doctor [--repair]
```

### Description

`doctor` prints one `ok` or `FAIL` line per check and exits nonzero when any check failed. Run it when searches look wrong or slow. The checks, in order:

| Check | What it verifies |
|-------|------------------|
| `config-loads` | the config file parses and, when it exists, sets at least one `dirs` entry. No config file is not a failure: the check passes and the report leads with the path to create |
| `repos-discovered` | a loaded config discovers at least one repo. Plain `ok` when every walked repo made it in; `ok` with the dropped counts (`N dropped by index.hosts (K with no remote), J excluded`) when a filter removed some; FAIL naming the filter when the walk found repos and dropped them all, or found none |
| `state-file-loads` | `state.json` parses; with `--repair`, a corrupt file is backed up and reset |
| `index-freshness` | every discovered repo's index matches its working tree; fails with the stale count. A machine with repos but none of them indexed yet passes with a note (the first search builds it), and when discovery yielded nothing this check steps aside with "no repos to check" rather than failing twice |
| `index-shards-valid` | every `.zoekt` shard opens cleanly |
| `search-server-responsive` | a running search server answers on its socket (a stopped server passes — it auto-starts) |
| `catalog-descriptors` | every catalog descriptor discovery found reads cleanly. Passes with a note when no repo carries one (`--owner`/`--system` lookups then match nothing); FAILs naming each repo and descriptor it couldn't read or follow; skipped when `repos-discovered` already failed |
| `web-ui-reachable` | the `csl web` process answers on its effective base URL — `web.base_url`, else `http://127.0.0.1:$CSL_PORT` (web-only; search works without it) |
| `web-ui-version-skew` | the running web process was built from the same commit as this binary (web-only) |

The same checks are available to an agent with no shell as the `csl_doctor` MCP tool, which returns the report as JSON and never repairs.

A stale-index FAIL self-heals: searches answer from the old shards and reindex in the background, or `csl index` does it synchronously. On a clean pass the last line points at `csl docs`, the embedded operating doc.

### Example output

```
ok config-loads
ok repos-discovered (17 repos, 2 dropped by index.hosts (1 with no remote))
ok state-file-loads
FAIL index-freshness: 3 of 17 repos stale, dirty, or unindexed; the next search reindexes them in the background, 'csl index' does it now
ok index-shards-valid
ok search-server-responsive
ok catalog-descriptors
ok web-ui-reachable
ok web-ui-version-skew
```

---

## `csl config`

Show which config file csl reads and the settings in effect.

### Synopsis

```sh
csl config
```

### Description

The first line names the resolved path and how it went — `loaded`, `missing, defaults in use`, or `parse error: <err>`. Below it is the config csl is actually running on: the file's values with every default filled in, printed as YAML in the same shape the file takes. A broken file is not fatal; the header says so and the defaults print anyway.

`csl config --help` carries an annotated example documenting every key, so a fresh machine can be configured without this page. Pair it with `doctor`: doctor shows the state csl resolved, config shows which file and key to change.

### Example output

```
config file: /Users/you/.config/csl/config.yaml (loaded)

dirs:
    - /Users/you/code/src
hooks:
    post_merge:
        enabled: false
        exclude: []
sync:
    concurrency: 8
index:
    hosts:
        - github.com
semantic:
    enabled: false
    sync: false
    ollama_url: http://localhost:11434
    embed_model: unclemusclez/jina-embeddings-v2-base-code:f16
    dim: 768
daemon:
    idle_timeout_minutes: 10
web:
    base_url: http://127.0.0.1:7424
```

The printed `web.base_url` is the effective one: with the key unset it is derived from `CSL_PORT`, so this line also tells you where the other surfaces think the UI is.

---

## `csl query`

Validate and parse a zoekt query without running it.

### Synopsis

```sh
csl query [--json] <pattern>
```

### Description

Useful when the query has escaped regex metacharacters and you want to confirm the parser sees what you mean. Returns the parsed tree on success, or the parse error plus a fixing hint on failure.

### Examples

```sh
csl query "func Walk"
# valid: true
# parsed: (and substr:"func Walk")

csl query "func \(Walk"
# valid: false
# error: ...
# hint: Escape special regex characters with backslash, e.g. "func \(Walk"
```

---

## `csl mcp`

Start the MCP stdio server.

### Synopsis

```sh
csl mcp
```

### Description

Reads JSON-RPC 2.0 requests from stdin, writes responses to stdout, and exits when stdin closes. The MCP client spawns one per session; there is no persistent server.

Register with Claude Code:

```sh
claude mcp add --scope user csl -- csl mcp
```

Smoke test without an MCP client:

```sh
( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}\n{"jsonrpc":"2.0","method":"notifications/initialized"}\n{"jsonrpc":"2.0","id":2,"method":"tools/list"}\n'; sleep 1 ) | csl mcp
```

Expected: an `initialize` response followed by `tools/list` listing ten `csl_*` tools with their input/output schemas. See [MCP reference](mcp.md) for per-tool details.

---

## `csl web`

Serve the code search web UI on localhost.

### Synopsis

```sh
csl web [--port <port>]
```

### Description

Starts an HTTP server bound to `127.0.0.1` only (never `0.0.0.0`), serving a
browser UI over the same zoekt index as `csl search`. The main page has a query
box and example queries; the Examples tab lists more. Results are grouped by repo
and file, link to the file on its git host (`/blob/HEAD/<file>#L<line>`), and can
be expanded inline. Searches use the daemon when available and fall back to an
in-process search otherwise.

The frontend is embedded in the binary. A JSON API backs the UI and is safe to
call directly:

- `GET /api/search?q=<query>&mode=files|content&repo=&lang=&file=&limit=&context=`
- `GET /api/read?repo=<name>&file=<path>&start=&end=`
- `GET /api/repos`
- `GET /healthz`

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `7424` | Port to listen on (loopback only). |

### Examples

```sh
csl web                       # serve on http://localhost:7424
csl web --port 8080           # serve on a different port

# Run as a background service via a process manager:
t-man add --name csl-web -- csl web --port 7424
```

See [Web UI](web.md) for the full UI and API reference.

---

## `csl docs`

Print the operating doc embedded in the binary, or the CLAUDE.md section.

### Synopsis

```sh
csl docs
csl docs --claude-md
csl docs --claude-md >> ~/.claude/CLAUDE.md
```

### Description

Without flags, prints `operating.md` rendered with the paths this install resolves (state directory, daemon log, web base URL): how csl runs, where state lives, failure modes, and first moves. With `--claude-md`, prints the section that teaches Claude Code when to reach for the `csl_*` MCP tools: csl-first for discovery and search, lexical by default with semantic and hybrid gated on the vector index, the repo health flow, and the zoekt query rules. The text is the fenced block under "Add this to your CLAUDE.md" in the README; a test holds the two byte-identical and checks the block names every tool `csl mcp` registers.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--claude-md` | `false` | Print the CLAUDE.md section instead of the operating doc |

---

## `csl shell-init`

Print the `repo` and `repo-sync` shell functions.

### Synopsis

```sh
csl shell-init zsh
csl shell-init bash
eval "$(csl shell-init zsh)"     # in ~/.zshrc
```

### Description

Prints two functions to stdout, one per line, ending with a newline:

```sh
repo() { local d=$(csl repo "$@"); [[ -n "$d" ]] && cd "$d"; }
repo-sync() { csl sync; }
```

`repo <query>` jumps to the single repo matching the query and bare `repo` opens the fuzzy picker; `repo-sync` runs `csl sync`. The bodies are the same ones the ralph recipe in this repo defines through `[shell.functions]`, and a test asserts they match, so a machine set up either way gets identical helpers. Any shell other than `zsh` or `bash` is rejected with a non-zero exit.

---

## `csl version`

Print the version string.

### Synopsis

```sh
csl version
csl version -o json
```

Plain output is the bare version token — the cross-tool convention sibling tools follow, so ralph and status can probe any of them for the build they are running. `-o json` prints the full build metadata object, every key present and `""` for anything unknown:

```json
{
  "version": "98b59d9",
  "commit": "98b59d992678fdf3b67f3e32911fae98d73165b2",
  "tag": "csl/v0.8.0",
  "build_time": "2026-08-13T19:47:44Z"
}
```

The four values are injected at build time from `buildinfo.mk` at the repo root, which links them into `github.com/mad01/thismoon/buildinfo`. `make build` and `make install` set all four; a `go install` build with no ldflags falls back to the vcs stamps the Go toolchain records.
