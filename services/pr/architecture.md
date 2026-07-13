# pr architecture

## Overview

pr is one Go process, `pr serve`, run as a launchd agent under t-man. It
listens on 127.0.0.1:7427 and is reached as `http://pr.this/` through d-man.
A background poller fetches open pull requests from the configured GitHub
repos across hosts, a JSON API serves the cached queue and live PR detail,
and review actions go back out the same way the data came in: every GitHub
call shells out to `gh api`, so pr holds no tokens and does no direct HTTP to
GitHub. The subprocess is also why the service runs without a sandbox
profile.

## Structure

```
cmd/pr/            entrypoint, delegates to internal/cli
internal/cli/      Cobra commands: serve, version; flag and env defaults
internal/config/   TOML config: poll_interval + [[source]] watch list
internal/github/   Client wrapping `gh api`, PR/diff/review/check types
internal/store/    on-disk PR cache with atomic writes and stale pruning
internal/server/   HTTP mux, Poller, diff parser, markdown rendering, shells
```

`internal/server` embeds four frontend files: `shell.html` + `app.js` for the
list page and `detail_shell.html` + `detail.js` for the detail page. Both
pages are chrome-only shells rendered in the browser from JSON, the
client-side rendering model of docs/adr/0005, with shared chrome mounted via
`webkit.Mount(mux)` from the in-module webkit Go package. `diff.go` parses
unified diffs into file/hunk/line structures and `markdown.go` renders PR
bodies; both run server-side so the detail API returns display-ready HTML.

## Data flow

Polling: `Poller.run` ticks every `poll_interval` (default 5m; `POST
/api/refresh` forces a pass). For each repo in the config,
`Client.ListPRs` runs `gh api repos/{owner}/{repo}/pulls?state=open` with
`GH_HOST` set to the source's host, which is the entire multi-host mechanism.
Results become `CachedPR` entries plus review decisions in the store; a
failed fetch keeps the previous cache alongside the error; an archived repo
(the pulls payload carries `base.repo.archived`) has its PRs dropped with no
extra call. After each pass the store prunes entries for repos no longer
configured and saves.

List page: `GET /` serves the shell, `app.js` fetches `GET /api/prs`, and the
handler serializes `Poller.Snapshot`, the cache joined back to the configured
repos.

Detail page: `GET /pr/{host}/{owner}/{repo}/{number}` serves the detail
shell, and `detail.js` fetches `GET /api/pr/...`, which bypasses the cache
and fetches live through `gh`: the PR itself, its unified diff (parsed and
rendered to HTML), reviews, and checks.

Actions: `POST /api/approve`, `/api/request-changes`, and `/api/merge` call
`gh api` to submit a review or squash-merge, then rely on the next poll to
reflect the result.

## Storage

One file: `store.json` under `--workdir` (default `~/.local/share/pr`), a
JSON map keyed `host/owner/repo` holding each repo's cached PRs, review
decisions, fetch time, last error, and archived flag. Writes go through a
temp file + rename so a crash never leaves a truncated store; entries older
than 24 hours are pruned at load, and entries for unconfigured repos are
pruned each poll. It is purely a cache — deleting it costs nothing but a cold
first render before the next poll completes.

## Interfaces

Web: `GET /` and `GET /pr/{host}/{owner}/{repo}/{number}` (shells), `GET
/api/prs` (cached queue), `GET /api/pr/...` (live detail), `POST
/api/approve`, `/api/request-changes`, `/api/merge`, `/api/refresh`, plus
`GET /healthz` (204), `GET /version` (ldflags-injected sha), and `/webkit/*`.

CLI: `pr serve` with `--port` (7427, env `PR_PORT`), `--config` (env
`PR_CONFIG`), and `--workdir` (env `PR_WORKDIR`); plus `pr version [-o json]`.

Config: `~/.config/pr/config.toml` with a `poll_interval` and `[[source]]`
blocks naming a host (github.com when omitted), an owner, and a repo list.
The file is machine-private and arrives via a companion recipe in the
consuming repo (docs/adr/0006); this directory ships no default. `gh` must be
installed and authenticated against every watched host.
