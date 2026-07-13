# pr, local PR review dashboard

Go CLI serving a PR review dashboard at `http://pr.this/` (port 7427).
Aggregates open pull requests from configured GitHub repos across multiple
hosts (github.com and GitHub Enterprise instances), shows diffs, and
supports approve/request-changes workflows. The list and detail pages are
chrome-only shells rendered client-side from JSON APIs (docs/adr/0005).

## Module layout

```
pr/
  cmd/pr/              entrypoint (delegates to internal/cli)
  internal/
    cli/               cobra: serve, version (Version via ldflags)
    config/            TOML config with per-host source lists
    github/            gh api wrapper (per-host GH_HOST), PR types
    store/             on-disk PR cache (store.json under --workdir), atomic writes, 24h stale-entry pruning
    server/            HTTP server, poller, diff parser, templates
  Makefile             part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

- **Config**: reads `~/.config/pr/config.toml` (host-gated via recipe).
  Each `[[source]]` block specifies a GitHub host, org/owner, and list of
  repos to watch.
- **Polling**: background goroutine fetches open PRs from all configured repos
  every `poll_interval` (default 5m). Results persist to `store.json` under
  `--workdir` (atomic writes, see `internal/store`). Archived repos
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

## Build / install / test

```bash
make build    # ./pr binary
make install  # build + cp to ~/code/bin/pr + adhoc codesign
make test     # go test ./...
```

## HTTP API

- `GET /`: PR list dashboard shell (client-rendered from `/api/prs`, polls every 5m)
- `GET /pr/{host}/{owner}/{repo}/{number}`: PR detail shell (client-rendered by `/detail.js` from `/api/pr/...`)
- `GET /detail.js`: detail-page client renderer
- `POST /api/approve`: approve a PR (JSON body)
- `POST /api/request-changes`: request changes (JSON body with message)
- `POST /api/merge`: merge/squash a PR
- `POST /api/refresh`: force re-poll all repos
- `GET /api/prs`: all cached PRs as JSON
- `GET /api/pr/{host}/{owner}/{repo}/{number}`: one PR's detail as JSON (live fetch; markdown body + diff pre-rendered to HTML)
- `GET /healthz`: 204
- `GET /version`: `{"version":"<sha>"}`
- `GET /webkit/`: shared chrome from the in-module `github.com/mad01/thismoon/webkit` package

## Shared UI: webkit

### How webkit is mounted
`internal/server/server.go` calls `webkit.Mount(mux)`, wiring the shared
chrome at `GET /webkit/`. webkit is an in-module package (no version pin);
the running binary always embeds the webkit committed beside it.

### Header markup
List page: `<wk-header brand="pr"></wk-header>`. Detail page:
`<wk-header brand="pr" back="/"></wk-header>`. The `back` attribute points
the header's back link at the list.

### Per-repo changes
`shell.html`/`detail_shell.html` layer pr-specific CSS variables
(`--pr-open`, `--pr-draft`, `--pr-additions`, `--pr-deletions`, review-state
colors) on top of the shared `webkit.css` palette. `app.js`/`detail.js`
build the DOM with `Webkit.el`/`Webkit.escapeHtml` per the client-side
rendering pattern (docs/adr/0005), with no server-side templating.

### Version check
`GET /version` reports the service's own build sha
(`{"version":"<sha>"}`). Drift in the shared chrome itself is checked via
webkit's own `GET /webkit/version` (asset-hash ETag); see `webkit/CLAUDE.md`.

## Gotchas

- **Not sandboxed.** This service shells out to `gh` (GitHub CLI) for all
  API calls. Sandboxing would block the subprocess execution. No sandbox
  profile.

## See also

- Recipe: `recipes/pr/recipe.toml` (+ `recipes/pr/CLAUDE.md`)
- Route: dotfiles `recipes/d-man/routes.toml` (`pr` -> 7427; machine overlay stays in dotfiles)
- Config: host-gated `~/.config/pr/config.toml` symlinks live in the dotfiles overlay (per-machine repo lists), the `pr-config` companion-recipe pattern cited in docs/adr/0006
