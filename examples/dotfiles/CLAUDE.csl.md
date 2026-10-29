# Code search across local checkouts: csl

csl (code-search-local) keeps a zoekt index of every git checkout under its configured directories and answers searches from that index. A question that spans many repositories ("which repos call this workflow", "which services pin this library", "where is this symbol defined across the fleet") is one csl call. The same question through `grep`, `rg`, `find` or `git grep` re-walks every checkout, takes minutes of CPU, and returns more text than the answer needs. These rules hold in any workspace where csl is present, as MCP tools or as a CLI.

## The rule

1. **Any search over more than one file goes through csl.** Pattern search, symbol lookup, "which files contain", "which repos have", counts per repo, listing a repo's files, reading a file by repo and path. This is the default, not a fallback.
2. **Fleet-wide discovery is where csl matters most.** Ask csl before you walk directories or loop `grep` over checkouts. Use `csl_count` grouped by repo for "how many repos", `csl_search` with a `f:` filter for "which repos have this file or line", and `csl_repo_lookup` with `owner`, `system` or `component` for catalog questions. One query across the fleet beats one process per repo.
3. **csl before any filesystem walk.** `find`, `ls -R`, `Glob` over a tree, `grep -r`, `rg`, `git grep` across checkouts, and scripts that walk directories are last resorts. Use them only when csl reports the repo is not indexed, or for one known file.
4. **Check the repo is indexed when in doubt.** `csl_repo_lookup` (or `csl repo <name>`) resolves a name to a checkout; an empty result means csl does not index it, and plain `grep` is then correct. Say so in the answer.
5. **Plain `grep` stays fine for filtering.** Piping another command's output through `grep`, `awk` or `jq`, or checking one line in one known file, is not a search and needs no csl.
6. **A `belt[prefer-csl]` hint is a directive.** When a hook answers a `grep` or `find` with a ready `csl_search` call, run that call next instead of another filesystem search. The hint already translated the pattern into zoekt syntax.
7. **Read large files in ranges.** `csl_outline` (or `csl outline`) for "how is this structured", then `csl_read` or `Read` with a line range for the region you need. A `belt[read-size]` hint names the outline call to use.

## Which form of csl you have

- **MCP tools present** (`mcp__csl__*` in the tool list): use them, and do not shell out to the `csl` CLI. The tools and the CLI share one daemon, so if one is down the other is too.
- **CLI only** (`csl` on PATH, no `mcp__csl__*` tools): run the CLI through Bash. Same index, same query language.
- **Neither**: say that csl is not available and fall back to `rg` with narrow globs. Prefer `rg --files -g <glob>` over `find`, and scope every search to the smallest directory that can hold the answer.

## Repo discovery and freshness

- Find repos with `csl_repo_lookup` or `csl_repo_info`. Do not hunt for repo directories with `find`, `ls`, `Glob` or shell loops.
- `csl_repo_lookup` takes a `name` (case-insensitive regex on org/repo) and `owner`, `system` and `component` filters. Those three read each repo's root catalog descriptor (`catalog-info.yaml` or `service-info.yaml`). A repo without a descriptor never matches them. Components nested inside a monorepo are not seen, so say when a count comes from root descriptors only.
- An empty lookup means the repo is not checked out locally, or sits outside csl's configured directories. Say so. Do not guess a path.
- A result with a non-empty `dropped` array means csl found the checkout but a config filter (`index.hosts` or an exclude list) removed it. Report that reason instead of calling the repo missing.
- `csl_repo_info` returns git health (branch, dirty files, index staleness) plus an `action` field (`ready`, `commit_or_stash`, `pull_recommended`, `needs_reindex`). Call it before changing a repo, and before telling the user something about its current state they will act on.
- Exploring needs no pull. Say "based on the local checkout, may be behind upstream".
- Never stash, reset or discard uncommitted work on your own. On a non-default branch, ask before switching.
- Stale and clean: pull with `csl_repo_pull` (it has dirty-state safety checks) and note it. After a pull that brought commits, re-run `csl_repo_info`; the earlier read is stale.
- `csl_repo_reindex` after significant local changes so search stays current.
- `csl_repo_health` for a fleet-wide sweep of uncommitted or unpushed work across every checkout. One call, not a loop of `csl_repo_info`.
- A search re-indexes a stale or dirty repo in the background after answering, so one search may answer from the previous commit. When freshness matters, call `csl_repo_info` and reindex first. `csl_index_info` reports when the index was last built.
- CLI equivalents: `csl repo <name>` (path of one repo), `csl repo --list --json`, `csl repo --owner <team>` (also `--system`, `--component`), `csl repo --skipped` (repos discovery dropped and why), `csl index` (reindex stale repos), `csl index --status`, `csl sync` (pull every clean repo and reindex), `csl doctor`.

## MCP tools

- Repo: `csl_repo_lookup` (name, `owner`, `system`, `component` filters), `csl_repo_info` (git health), `csl_repo_health` (fleet sweep), `csl_repo_pull`, `csl_repo_reindex`.
- Search: `csl_search` (default; `output_mode: content` for lines, `file` and `lang` to narrow, `limit` and `offset` to page), `csl_count` (totals, or per repo with `group_by: repo`), `csl_query_validate`.
- Semantic: `csl_semantic_search`, `csl_hybrid_search`, only when `csl_index_info` reports the semantic index built; otherwise stay on `csl_search`.
- Read and orient: `csl_read` (file by repo and path, with `start_line` and `end_line`), `csl_ls`, `csl_outline`, `csl_show_file` (opens a section for the user), `csl_index_info`.
- Diagnosis: `csl_doctor`. Call it before retrying a failed call.

## CLI equivalents

```sh
csl repo <name>                                   # resolve one repo to its path
csl repo --list --json                            # every indexed repo
csl repo --owner <team>                           # repos by catalog owner (also --system, --component)
csl repo --skipped                                # repos discovery dropped and why
csl index                                         # reindex stale repos
csl index --status                                # when each repo was last indexed
csl sync                                          # pull every clean repo and reindex
csl search '<query>' --repo <re> --file '<re>'    # files that match (default)
csl search '<query>' -o content -C 2 --limit 100  # matching lines with context
csl count '<query>' --group-by repo               # how many matches per repo
csl count '<query>' --lang go --repo <re>         # one total, narrowed
csl read <path> --repo <name> --start-line N --end-line M
csl outline <repo> [path] --kinds struct,function # definitions ranked by use
csl query '<query>'                               # how zoekt parsed it
csl doctor                                        # when a call errors or comes back empty
```

Every search flag is a regex, including `--repo` and `--file`. Add `--json` to parse the output.

## Query syntax: zoekt, not grep

- Two space-separated terms are AND within one file. Keep it to one or two terms and narrow with filters; three or more terms usually return nothing.
- OR is `a|b` with no spaces, or lowercase `or`. Uppercase `OR` is a literal. `a | b` is three AND terms.
- Filters: `repo:<re>` (not `r:`), `f:<re>` (not `file:`; regex, not glob, so `f:\.go$`), `lang:go`, `-term` to exclude, `case:yes`.
- `sym:Name` matches definitions only (functions, types, methods, classes) and skips call sites. Use it for "where is X defined". Two names: `sym:Foo|Bar`.
- Exact phrase: `"foo bar"` on one line. Proximity: `foo.*bar`.
- Zero results carry a hint: how the query parsed, how many files each term matches alone, and the term to drop. Read it before retrying. A query refused with a fix means apply the fix, not resend it.
- Translation: `grep -r foo` is `foo`; `grep --include='*.go' foo` is `foo f:\.go$`; `grep -v test` is `foo -test` or `-f:_test\.go`; `grep -E 'a\|b'` is `a|b`; `find . -name '*.yaml' | xargs grep foo` is `foo f:\.ya?ml$`.

## Fleet questions, worked

- Which repos use a reusable workflow: `csl_search` query `uses:.*<workflow-file>` with `f:\.github/workflows/`, `limit` high enough to page through, then count distinct repos from the paths. Or `csl_count` on the same query with `group_by: repo`.
- Which repos carry a file: `csl_search` with a term that every such file has (`name`, `version`, `apiVersion`) and `f:<filename>$`; the file list is the answer.
- Where a symbol is defined across the fleet: `csl_search` query `sym:Name`, `lang` set.
- Who owns which repos: `csl_repo_lookup` with `owner`; it reads each repo's catalog descriptor.
- A value per repo (a version, an option): `csl_search` in content mode with the key as the query, then read the value off each line. Do not open the files one by one.
- Ranking many candidates (every workflow, every library, every option value): one search over the common pattern, then aggregate the lines yourself. Never loop one `csl_count` or `csl_search` per candidate; a hundred small calls cost more turns and tokens than one wide one.
- When that one search returns thousands of lines, do not page the MCP tool with `offset`: an MCP result is capped per call and the pages pile up in context. Run the CLI through Bash instead and aggregate in the pipe, with `--limit` high enough for the whole result:
  `csl search '<pattern>' -o content --limit 5000 --file '<re>' | awk -F/ '{print $2}' | sort | uniq -c | sort -rn`. The MCP tools are for lookups, reads and counts whose answer fits in one call.

Report fleet counts as distinct repositories, not matching files or lines, unless asked otherwise.
