# pr — local PR review dashboard

Go CLI serving a PR review dashboard at `http://pr.this/` (port 7427).
Aggregates open pull requests from configured GitHub repos across multiple
hosts (github.com and GitHub Enterprise instances), shows diffs, and
supports approve/request-changes workflows.

## Module layout

```
pr/
  cmd/pr/              entrypoint (delegates to internal/cli)
  internal/
    cli/               cobra: serve, version (Version via ldflags)
    config/            TOML config with per-host source lists
    github/            gh api wrapper (per-host GH_HOST), PR types
    server/            HTTP server, poller, diff parser, templates
  Makefile             part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

- **Config**: reads `~/.config/pr/config.toml` (host-gated via recipe).
  Each `[[source]]` block specifies a GitHub host, org/owner, and list of
  repos to watch.
- **Polling**: background goroutine fetches open PRs from all configured repos
  every `poll_interval` (default 5m). Results cached in memory. Archived repos
  are excluded: the pulls payload carries `base.repo.archived`, so an archived
  repo's PRs are dropped (and its stale cache overwritten) with no extra API
  call.
- **GitHub API**: all calls go through `gh api` with `GH_HOST=<host>` set per
  source. No direct HTTP client; relies on `gh` being authenticated.
- **Diff rendering**: fetches unified diff from GitHub API, parses into
  file/hunk/line structures, renders as HTML with line numbers and
  add/del/context coloring.
- **Actions**: approve and request-changes call `gh api` to submit reviews.
  Merge (squash) also supported.

## Endpoints

- `GET /` — PR list dashboard shell (client-rendered from `/api/prs`, polls every 5m)
- `GET /pr/{host}/{owner}/{repo}/{number}` — PR detail shell (client-rendered by `/detail.js` from `/api/pr/...`)
- `GET /detail.js` — detail-page client renderer
- `POST /api/approve` — approve a PR (JSON body)
- `POST /api/request-changes` — request changes (JSON body with message)
- `POST /api/merge` — merge/squash a PR
- `POST /api/refresh` — force re-poll all repos
- `GET /api/prs` — all cached PRs as JSON
- `GET /api/pr/{host}/{owner}/{repo}/{number}` — one PR's detail as JSON (live fetch; markdown body + diff pre-rendered to HTML)
- `GET /healthz` — 204
- `GET /version` — `{"version":"<sha>"}`
- `GET /webkit/` — shared chrome from the in-module `github.com/mad01/thismoon/webkit` package

## Build / install / test

```bash
make build    # ./pr binary
make install  # build + cp to ~/code/bin/pr + adhoc codesign
make test     # go test ./...
```

## Not sandboxed

This service shells out to `gh` (GitHub CLI) for all API calls. Sandboxing
would block the subprocess execution. No sandbox profile.

## See also

- Recipe: `recipes/pr/recipe.toml` (+ `recipes/pr/CLAUDE.md`)
- Route: dotfiles `recipes/d-man/routes.toml` (`pr` -> 7427; machine overlay stays in dotfiles)
- Config: host-gated `~/.config/pr/config.toml` symlinks live in the dotfiles overlay (per-machine repo lists)
