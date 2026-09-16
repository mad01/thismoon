# operating csl

csl is local code search: it indexes every git checkout under the configured
directories with zoekt and answers queries from one binary through three
surfaces: the `csl` CLI, an MCP stdio server (`csl mcp`), and a loopback web
UI (`csl web`). Lexical search always works; semantic and hybrid search are
optional and backed by a local Ollama server.

## how it runs

Nothing has to be running for search to work. Every query path (CLI, MCP
tool, web API) pings the background search server on its Unix socket and, if
nothing answers, forks `csl search --serve` as a detached process. That
server keeps the zoekt shards mmap'd across queries and idle-exits after 10
minutes (`daemon.idle_timeout_minutes`). If it cannot be reached at all, the
caller opens the shards in-process for that one query, so a dead search
server slows queries down but does not fail them.

`csl mcp` is a stdio shim spawned per session by the MCP client. The shim
being up says nothing about the search server; each tool call runs the same
ping-then-fallback path. The only long-lived process is the web UI,
`csl web` (default {{.BaseURL}}), typically supervised by t-man as the
csl-web agent.

MCP tools answer in `text` by default: csl_search prints ripgrep-style
headings with `LINE:match` lines, csl_read prints `LINE:text`, and tools
without a renderer of their own fall back to `key: value` lines. Pass
`response_format` on a call (`json` for the structured object; `jsonl`, `csv`,
`markdown-kv`, `xml`, or `toon`, Token-Oriented Object Notation) or set
`mcp.response_format` in config.yaml; the parameter wins, and an unknown value
in either place is an error that names its source.

`csl outline <repo> [path]` and the csl_outline tool are the one query path
that skips the index: they walk the working tree with the indexer's skip
rules, extract definitions with the tree-sitter grammars, and rank them by
how many other files hold the name as a whole identifier. Nothing has to be
indexed or running, and the outline is always current. An outline that looks
thin is usually the scope: fields, enumerators, and markdown headings are out
unless `kinds` names them (the empty-result line says how many were skipped),
test files are out unless include_tests is set, only languages with a grammar
(Go, TypeScript, Python, Java, protobuf, bash, markdown) contribute
definitions, and a `path` narrows the definitions but never the reference
count. A `files_capped` result stopped at max_files (20000) and covers the
first files in path order only.

## where state lives

`config.yaml` sits in the config directory (`--config`, else CSL_CONFIG, else
~/.config/csl); `csl config` prints the path it resolved and the settings in
effect. The file is optional: with none, csl runs on defaults with no repos
configured and every surface says which file to create. A file that exists but
does not parse is an error. Everything csl writes sits under {{.StorePath}}:

- `search-index/`: the lexical index. `state.json` records each repo's
  fingerprint; `<hash>.zoekt` shard files sit beside it.
- `semantic-index/`: per-repo vector stores, present only after a
  `csl index --semantic-all` run.
- `search-daemon.sock` / `.pid` / `.log`: the search server's socket, PID
  file, and rotated log.
- `reindex.queue`: repo paths queued for reindex, drained by `csl sync`.

## failure modes

Start with `csl doctor` (or the csl_doctor tool, same checks as JSON): one
ok/FAIL line per check — config, repos discovered, state file, index
freshness, shard integrity, and search-server responsiveness, plus two
web-only checks (web-ui-reachable, web-ui-version-skew) that can fail while
search keeps working. The config check fails on a file that does not parse,
or one that parses and sets no `dirs` — the usual cause of "csl finds nothing
at all". repos-discovered fails when a loaded config discovers nothing and,
when a filter dropped everything, says so with a count per filter. No config
file at all is not a failure: the check reports ok with the path to create
beside it. `--repair` resets a corrupt state file; the tool never repairs.

Empty search result: usually the query, not an error. zoekt AND requires all
space-separated terms in the SAME file, so 3+ terms almost always return
zero. Use 1-2 terms plus repo:/f:/lang: filters. OR is `|` with no spaces
(`a | b` is three AND terms; uppercase OR is a literal). `f:` takes a regex,
not a glob (`f:\.go$`). csl_search's zero_result_hint counts files per AND
term (term_counts) and reruns once without the terms that match nothing;
that result carries relaxed_query and dropped_terms. A query of only
quotes or with an unbalanced quote is refused with the fix. Run `csl query
"<pattern>"` (or csl_query_validate) to see how the query parsed and the
term split. If the query is fine, the repo may not be
indexed: `csl repo <name> --list` resolves it, and an empty result means the
repo is not checked out, not under the configured dirs, or dropped by
index.hosts or the exclude list (`csl repo --list --skipped` says which, and why).

Stale index: a repo's fingerprint (HEAD, branch, dirty state) no longer
matches `state.json`. Searches still answer from the old shards, then
reindex stale repos in the background, so the next query is current. Force
it with `csl index` (stale repos only) or `csl sync` (pull + reindex).
`csl index --status` and csl_index_info show staleness counts.

Semantic or hybrid unavailable: `available=false` (hybrid degrades to
lexical-only) until `csl index --semantic-all` has run once and Ollama is up
with the configured embedding model pulled. `csl config` shows the model in
effect. The csl_semantic_search and csl_hybrid_search tools are registered
only when `semantic.enabled` is true, so a client that lists neither is
looking at a machine with semantic search turned off.

Web UI unreachable: t-man typically supervises it. Run `t-man list` to see
whether the csl-web agent exists, then `t-man restart csl-web`. For a quick
test without t-man, `csl web` in a spare terminal also works.

No owner/system on a repo, or --owner/--system finds nothing: check `csl
doctor` (catalog-descriptors) and `csl repo --json <name>`. The descriptor
(catalog-info.yaml or service-info.yaml) must sit at the repo root, or a root
.csl-catalog.yaml must point at it.

## version skew

`csl version -o json` reports the binary on PATH; `GET {{.BaseURL}}/version`
reports the running web process. `csl doctor` runs the comparison as its
web-ui-version-skew check. When the `commit` values differ, an old
process survived an upgrade: `t-man restart csl-web`. Skew that survives a
restart means the plist names a versioned path (a mise install dir); re-add
the agent with the mise shim path. The background search
server can also be an old build; `csl search --stop` kills it, and the next
query forks a fresh one from the current binary.

## first moves

1. `csl doctor` (config, state, freshness, shards, search server, web UI)
2. Zero results: `csl query "<pattern>"` to see the parse, then retry with
   1-2 terms and filters
3. `csl repo <name> --list` to confirm the repo is discovered at all
4. Compare `csl version -o json` with `GET {{.BaseURL}}/version` for skew
5. Tail {{.LogPath}} if queries keep falling back to in-process search
