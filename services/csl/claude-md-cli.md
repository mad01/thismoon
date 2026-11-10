## Code search across local checkouts (csl)

csl (code-search-local) keeps a zoekt index of every git checkout under its configured directories and answers searches from that index. A question that spans many repositories ("which repos call this workflow", "which services pin this library", "where is this symbol defined across the fleet") is one `csl` call through Bash. The same question through `grep`, `rg`, `find` or `git grep` re-walks every checkout, takes minutes of CPU, and returns more text than the answer needs. These rules hold wherever the `csl` CLI is on PATH; the MCP tools are not registered here, so every call is a shell command.

### The rule

1. **Any search over more than one file goes through csl.** Pattern search, symbol lookup, "which files contain", "which repos have", counts per repo, listing a repo's files, reading a file by repo and path. csl is the default, and the filesystem tools are the fallback.
2. **Fleet-wide discovery is where csl matters most.** Ask csl before you walk directories or loop `grep` over checkouts. One search piped through `sort | uniq -c` answers "which repos have this", `csl count '<pattern>' --group-by repo` answers "how many per repo", and `csl repo --owner <team>` (also `--system`, `--component`) answers catalog questions.
3. **csl before any filesystem walk.** `find`, `ls -R`, `Glob` over a tree, `grep -r`, `rg`, `git grep` across checkouts, and scripts that walk directories are last resorts. Use them only when csl reports the repo is not indexed, or to read one or two known files.
4. **Check the repo is indexed when in doubt.** `csl repo <name>` resolves a name to a checkout. An empty result means csl does not index it, and plain `grep` is then correct. Say so in the answer.
5. **Plain `grep` stays fine for filtering.** Piping csl's output through `grep`, `awk` or `jq`, or checking a line in one or two known files, is filtering and needs no csl.
6. **Size the limit to the answer.** `csl search --limit` caps files (default 50), not repos: when the output has as many lines as the limit, rerun with a higher one before trusting the list. For "which repos" use `csl count '<pattern>' --group-by repo --names`, which has no cap. Aggregate wide results in the pipe (`cut`, `awk`, `sort | uniq -c`) so only the summary reaches the context.
7. **Ask about large files; read only what you will edit.** A question over a big file or many files goes to `csl ask '<question>' -q '<query>'` first: a cheap worker model reads the files the query selects, and only its answer reaches your context. A ranged read is for a region you will edit: `csl outline <repo> [path]` for the structure, then `csl read <path> --repo <name> --start-line N --end-line M` or `Read` with a line range.
8. **Always `set -o pipefail` in a csl pipe**, so a csl failure (no index, bad pattern) is an error and not an empty list that reads as "no matches".
9. **A miss explains itself on stderr.** Stdout prints `no matches`, and stderr prints the parsed query, the repos searched, the index age and the files each AND term matches alone. Read it before retrying. When some AND terms match no file and others do, csl reruns once without them and marks the rerun with a `relaxed:` line. A malformed query, such as a lone quote, exits 1 with the fix: apply it, do not resend.

### Repo discovery and freshness

- Find repos with `csl repo <name>` (a case-insensitive regex on org/repo) or `csl repo --list`. Do not hunt for repo directories with `find`, `ls`, `Glob` or shell loops.
- `csl repo --owner <team>`, `--system` and `--component` read each repo's root catalog descriptor (`catalog-info.yaml` or `service-info.yaml`), so a repo without one never matches them. Components nested inside a monorepo are not seen; say when a count comes from root descriptors only.
- `csl repo --skipped` lists the checkouts discovery dropped and why, such as a host filter or an exclude list. Report that reason instead of calling the repo missing.
- Exploring needs no pull. Say "based on the local checkout, may be behind upstream".
- Never stash, reset or discard uncommitted work on your own. On a non-default branch, ask before switching.
- `csl index --status` shows when each repo was last indexed; `csl index` reindexes the stale ones; `csl sync` pulls every clean repo and reindexes. A search re-indexes a stale or dirty repo in the background after answering, so one search may answer from the previous commit. When freshness matters, run `csl index` first.
- **A linked worktree is not in the main index**, which holds each repo's main checkout. Working in one (under `~/.worktrees`, say), pass `--worktree .` (or its path) to `csl search`, `count`, `read`, `outline` or `ask`: the command answers from the worktree's own index, rebuilt when its files change, and names the repo `org/repo@branch`. Without it the working directory narrows nothing, so a fleet search from a worktree still covers the fleet. A `--repo` that selects only the worktree's repo (`'thismoon$'`) also answers from the worktree and says so on stderr; `--no-worktree` keeps it on the main index.
- `csl doctor` when a call errors or comes back empty. Run it before retrying.

### CLI

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
csl search '<query>' --limit 5000 | cut -d/ -f1-2 | sort | uniq -c | sort -rn  # matching files per repo
csl search '<query>' --worktree <path>            # one linked worktree, from its own index
csl count '<query>' --group-by repo               # how many matches per repo
csl count '<query>' --group-by repo --names       # every matching repo, one per line, no cap
csl count '<query>' --lang go --repo <re>         # one total, narrowed
csl read <path> --repo <name> --start-line N --end-line M
csl ask '<question>' -q '<query>' --repo <re>     # a worker model answers over the matching files
csl ask '<question>' -q '<q1>' -q '<q2>' --per-repo # one answer per repo
csl search '<query>' --limit 5000 | csl ask '<question>' --files-from -
csl outline <repo> [path] --kinds struct,function # definitions ranked by use
csl query '<query>'                               # how zoekt parsed it
csl doctor                                        # when a call errors or comes back empty
```

Every search flag is a regex, including `--repo` and `--file`. A file result prints as `org/repo/path` and a content line as `LINE:COL:text` under its file, so `cut -d/ -f1-2` yields the repo. Add `--json` to parse the output instead.

### Query syntax: zoekt, not grep

- Two space-separated terms are AND within one file. Keep it to one or two terms and narrow with filters; three or more terms usually return nothing.
- OR is `a|b` with no spaces, or lowercase `or`. Uppercase `OR` is a literal. `a | b` is three AND terms.
- Filters: `repo:<re>` (not `r:`), `f:<re>` (not `file:`; regex, not glob, so `f:\.go$`), `lang:go`, `-term` to exclude, `case:yes`.
- `sym:Name` matches definitions only (functions, types, methods, classes) and skips call sites. Use it for "where is X defined". Two names: `sym:Foo|Bar`.
- Exact phrase: `"foo bar"` on one line. Proximity: `foo.*bar`.
- A search with zero results carries a hint: how the query parsed, how many files each term matches alone, and the term to drop. Read it before retrying. A query refused with a fix means apply the fix, not resend it.
- Translation: `grep -r foo` is `foo`; `grep --include='*.go' foo` is `foo f:\.go$`; `grep -v test` is `foo -test` or `-f:_test\.go`; `grep -E 'a|b'` is `a|b`; `find . -name '*.yaml' | xargs grep foo` is `foo f:\.ya?ml$`.

Before the first query:

- Start with one distinctive term plus the filter that scopes it (`--repo`, `--file`, `--lang`). Add a second AND term only to cut noise; a third must also sit in the same file and usually returns nothing. Join alternatives as `a|b` instead.
- Pick the form by the question: a plain term for usages, `sym:Name` for where a thing is defined, `f:<name>$` for a file by name, `csl ask` for what the files say rather than where they are.
- Every term is a regex, so escape a metacharacter you mean literally: `fmt\.Errorf`, `Greet\(`, `\[\]byte`.
- A `"quoted phrase"` matches only that exact text; drop the quotes to match the words as separate AND terms.
- On a miss, the file filter is the most common over-constraint: drop it before loosening the query. Then read the hint, fix the trap it names, and `csl query '<query>'` to see the parse.

### Fleet questions, worked

- Which repos use a reusable workflow: `csl search 'uses:.*<workflow-file>' --file '\.github/workflows/' --limit 5000 | cut -d/ -f1-2 | sort -u`. The line count is the repo count.
- Which repos carry a file: `csl search 'f:(^|/)<filename>$' --limit 5000 | cut -d/ -f1-2 | sort -u`.
- Where a symbol is defined across the fleet: `csl search 'sym:<Name>' --lang <lang>`. The answer fits one call.
- Who owns which repos: `csl repo --owner <team>`; it reads each repo's catalog descriptor.
- A value per repo (a version, an option): `csl search '<key>' -o content --file '<re>' --limit 5000`, then pull the value out in the pipe with `grep -o` or `awk`. Do not open the files one by one.
- What many files say, not where they are (how each service sets its timeouts, what every caller passes): `csl ask '<question>' -q '<query>' --per-repo`. The answer cites `path:line`, sometimes by the bare file name; find the full path in the corpus list and read those lines before editing them.
- Ranking many candidates (every workflow, every library, every option value): one search over the common pattern, aggregated in the pipe: `csl search '<pattern>' -o content --file '<re>' --limit 5000 | grep -o '<value-re>' | sort | uniq -c | sort -rn`. Never loop one `csl count` or `csl search` per candidate; a hundred small calls cost more turns and tokens than one wide one.

Report fleet counts as distinct repositories, not matching files or lines, unless asked otherwise.
