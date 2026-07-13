# why csl

## The problem

A single developer machine accumulates dozens of git checkouts across a few
source directories. Answering "where is this function defined" or "which repos
call this API" with `grep -r` over all of them is slow, and for a coding agent
it is worse: the default reach for `find`, `ls`, and raw `grep` guesses at
paths, misses repos, and dumps walls of output into the context window. Repo
discovery has the same shape: "is this repo checked out, on what branch, is
it dirty, is its index current" are questions an agent otherwise answers by
guessing.

## Why its own service

Search is the substrate the rest of the platform's code work leans on, not a
feature of any one component; csl predates the monorepo and came in as a clean
import (see `docs/MIGRATED-FROM.md`). Hosted code search would mean shipping
local checkouts to a cloud service, which the platform's local-only stance
rules out. zoekt itself is an engine, not a finished tool: csl adds the parts
an agent and a human actually need on top of it: repo discovery and host
filtering, index freshness tracking, a CLI, a localhost web UI, and an MCP
server so agents search, resolve repo paths, and read files without shelling
out.

## Why this shape

Three decisions carry the design. The index is zoekt's trigram index:
grep-like queries across dozens of checkouts return in milliseconds, and every
surface (CLI, MCP, web) is a thin shell over the same internal search code.
Queries go through a short-lived background search process spawned on
demand from the same executable: it keeps the zoekt shards mmap'd across
calls so repeat queries are fast, idle-exits after ten minutes, and every
caller falls back to opening shards in-process if it is unreachable — there is
no separately managed server. Freshness is a per-repo fingerprint of
HEAD, branch, and `git status`; a stale repo is reindexed in the background
after results return, so the current query stays fast and the next one
reflects the latest state. A consequence worth naming: code chunking compiles
tree-sitter grammars via cgo, so csl builds from a checkout only and its
release-artifact entry is a deliberate no-op; the fleet installs it from
source.

## Non-goals

csl never writes to a repo beyond `csl sync` / `csl_repo_pull`, which are
fast-forward-only pulls with dirty-tree safety checks; it does not commit,
branch, or edit. It does not index repos that are not checked out locally, and
an empty lookup means exactly that. Semantic and hybrid search are optional:
they need Ollama and an explicit index build, and lexical search never depends
on them. It no longer manages git hooks — suspenders owns those, and csl only
drains the reindex queue they feed.
