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
- Definitions only: `sym:Name` matches symbol definitions (function, type, method, class names) and skips call sites and comments; in content mode each hit carries its `kind`. Plain queries already rank the defining file first.
- File filters are regex, not glob: `f:.*\.go$` not `f:*.go`.
- Exact phrase: `"foo bar"` requires that exact string on one line. For proximity, use regex: `foo.*bar`.
- Validate: call `csl_query_validate` to see how zoekt parsed your query. This is especially useful when you get zero results.
- Zero results: read `zero_result_hint`. `term_counts` says how many files each AND term matches on its own; a `0` is the term to drop. When `csl_search` already dropped it and reran, the response carries `relaxed_query` and `dropped_terms`: use those results and do not add the term back. An empty, quote-only, or unbalanced-quote query is refused with the fix.

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
