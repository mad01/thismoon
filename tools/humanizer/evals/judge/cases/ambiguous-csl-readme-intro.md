---
id: ambiguous-csl-readme-intro
label: unknown
bucket: ambiguous
source: thismoon services/csl/README.md lines 1-9, commit 2a9cea106725 (2026-09-30)
license: BSD-3-Clause (this repository)
generator: ""
words: 220
notes: AI-drafted then humanized component README intro plus How it works (MAD-337/MAD-340).
---
# csl

Local code search over your git checkouts. A CLI, a localhost web UI, and an MCP stdio server for Claude Code, all backed by [zoekt](https://github.com/sourcegraph/zoekt). For people who want fast cross-repo `grep`, and for agents that need to search, resolve repo paths, and read files without shelling out.

`csl` walks the directories you configure, indexes every git repo it finds, and searches them with zoekt's trigram index. You get `grep`-like queries across dozens of checkouts in milliseconds, without shipping your code to a cloud service. A short-lived search daemon spawns on demand from the same binary, keeps zoekt shards mmap'd across queries, and exits after ten minutes of idleness. There is no server to manage separately.

## How it works

The CLI, `csl mcp`, `csl web`, and the search daemon share the same internal search/index code. A query first tries the daemon over a Unix socket (auto-started if not running); if that fails, it falls back to opening the zoekt shards in-process. Index freshness is tracked per repo by a fingerprint of `HEAD` + branch + `git status`, so stale repos get reindexed in the background after results come back. `csl hybrid` / `csl_hybrid_search` add a second backend (vector embeddings over the same repos, searched by meaning) and fuse it with the lexical results by Reciprocal Rank Fusion.
