# prs architecture

One binary, four roles: HTTP server + poller (`prs serve`, the single
writer), MCP stdio server (`prs mcp`), CLI client (`prs list` and friends),
and the embedded web page. Everything except serve is a thin HTTP client.

```
                    ┌────────────────────────────────────────────┐
  gh auth token ──► │ prs serve (t-man agent, port 7427)         │
  (per host, once)  │                                            │
                    │  poller: repofind walk ─► poll targets     │
  GitHub REST  ◄──── │          ─► go-github fetch (8 workers)    │
  (per host)        │          ─► store.SetRepos                 │
                    │  store:  in-memory map + repos.jsonl       │
                    │  server: /api/prs /api/status /api/refresh │
                    └────────────────▲───────────────────────────┘
                                     │ HTTP (localhost only)
              ┌──────────────────────┼──────────────────────┐
         prs mcp (stdio)        prs CLI                web page
         prs_list/refresh/…     list/refresh/status    shell.html + app.js
```

## data flow, one cycle

1. `poller.Refresh` walks the config `dirs` with `kit/repofind` (32-worker
   filesystem walk, origin remote parsed into host + org/repo).
2. `pollTargets` filters: no-remote repos out, `hosts` allowlist, exclude
   globs (config + built-in defaults) with `include` globs overriding, and
   duplicate checkouts of one remote collapse.
3. 8 fetch workers call `github.Client.OpenPRs` per target: one list-pulls
   call, then one list-reviews call per open PR, reduced to a standing
   review decision. Drafts drop here.
4. On error the repo keeps its previous PRs with the error recorded; the
   store appends only records whose content changed, plus tombstones for
   repos no longer discovered.

## packages

| Package | Owns |
|---|---|
| `internal/config` | YAML load, glob/interval validation, defaults, the built-in exclude list |
| `internal/github` | gh token minting + cache (`token.go`), go-github calls + review reduction (`client.go`) |
| `internal/poller` | discovery, target filtering, worker fan-out, staleness loop |
| `internal/store` | PR/RepoState model, filter/sort/facets, JSONL last-wins log with compaction |
| `internal/server` | routes, param validation, embedded shell.html + app.js |
| `internal/client` | HTTP client used by CLI and MCP, error chokepoint with the doctor hint |
| `internal/mcpserver` | the three `prs_*` tools over `internal/client` |
| `internal/cli` | cobra wiring, flag/env resolution, docs/doctor from kit/agentdoc |

## interfaces (the seams)

- `poller.Fetcher` — what the poller needs from GitHub; `*github.Client`
  satisfies it, tests inject a fake.
- `server.Refresher` — what the server needs from the poller; tests inject a
  canned summary.
- `TokenSource.mint` — the gh exec, injected in tests.
- `Client.apiBase` — host → REST base URL, pointed at httptest servers in
  tests.

## storage

`~/.local/share/prs/repos.jsonl`, append-only, one `RepoState` per line.
Load resolves newest-per-repo (greater `fetched_at`, tie → later line).
Quiet cycles append nothing; startup compacts when the file exceeds 100
lines and 4× the live repo count; a torn final line is truncated away. The
whole file is a disposable cache of GitHub state.
