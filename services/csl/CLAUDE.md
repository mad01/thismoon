# CLAUDE.md — csl

Local code-search daemon and MCP server backed by zoekt. Indexes local Git repos
and exposes them over a stdio MCP interface and a localhost web UI.

## Module layout

```
cmd/csl/           — entrypoint
internal/
  cli/             — Cobra commands (web, mcp, index, search, repo, doctor, version, …)
  daemon/          — search daemon lifecycle (socket, pid, log)
  search/          — zoekt indexer/searcher wrappers
  queue/           — background reindex queue
  repo/            — repo discovery + host routing
  mcpserver/       — MCP stdio server wiring (csl_* tools)
  web/             — HTTP shell + embedded frontend (assets/index.html, app.js, app.css)
Makefile
```

Part of the thismoon single Go module — no go.mod here; packages live under
`github.com/mad01/thismoon/services/csl/...`.

## Build / install / test

```bash
make build    # ./csl binary
make install  # build + cp to ~/code/bin/csl + codesign
make test     # go test ./...
make lint     # golangci-lint run ./...
```

## Shared UI: webkit

The chrome (header, theme toggle, font/size/bionic controls) comes from
**`github.com/mad01/thismoon/webkit`** — the in-module package at the repo root
(`webkit/`) that embeds compiled TypeScript/CSS web components. csl compiles
against the webkit committed beside it; there is no version pin. Do NOT re-add
palette, topbar, or theme CSS locally; those live in webkit only.

### How webkit is mounted

```go
import "github.com/mad01/thismoon/webkit"
mux.Handle("GET /webkit/", webkit.Handler()) // serves /webkit/webkit.css + .js
```

Templates inline the FOUC guard, load webkit assets, and use `<wk-header>`:

```html
<head>
  <script>(function(){try{var t=localStorage.getItem('webkit-theme')||'light';document.documentElement.setAttribute('data-theme',t);var s=parseInt(localStorage.getItem('webkit-size'),10);if(!isNaN(s)){s=Math.max(12,Math.min(24,s));document.documentElement.style.fontSize=s+'px';}}catch(e){}})();</script>
  <link rel="stylesheet" href="/webkit/webkit.css">
</head>
```

### Header markup (csl)

`index.html` uses:

```html
<wk-header brand="csl·search">
  <a data-nav class="active" href="/">Search</a>
</wk-header>
```

`webkit.js` injects the full control set (font · bionic · size ± · reload · theme)
automatically — do not add those controls manually.

### Per-repo changes

- Header markup lives in `internal/web/assets/index.html`.
- webkit components csl uses:
  - `<wk-header brand="csl·search">` with a `[data-nav]` Search link
  - `<wk-page-header>`, `<wk-title>`, `<wk-subtitle>` for the hero block
  - `<wk-search>` with a leading `<svg>` icon for the main query input
  - `<wk-seg>` for the Files / Matches view toggle
  - `<wk-badge variant="filter" [active]>` for active filter pills
  - `<wk-table>` for tabular result views
- csl-bespoke CSS (kept in `app.css`, not part of webkit): result rendering
  (`.repo-group`, `.file-group`, `.line`, `.match`), result quick filters
  (`.result-facets`, `.facet-count`), status row with view toggle
  (`.status-row`, `.status`), pagination
  (`.load-more`), the landing-page example guide shown in the empty state
  (`#examples`, `.ex-section`, `.ex-q`, `.qhl` — data + render in `app.js`),
  and zoekt query syntax highlighting on both the search input overlay and the
  example queries (`.search-box`, `.search-hl`, `.qhl`, `.t-*` token classes —
  tokenizer in `app.js`).
- Theme changes fire `document` event `wk-themechange` with `detail.theme` —
  listen there (not `onThemeChange` callback) if a page has dynamic visuals.
- `[data-nav]` children of `<wk-header>` become nav links; `[data-extra]`
  children become app-specific buttons in the controls area.

## Gotchas

- **`app.js` no longer owns theme.** Theme toggling and persistence are handled
  entirely by `webkit.js` + `<wk-header>`. `app.js` should not duplicate that logic.
- The MCP server (`csl mcp`) is registered in
  `dotfiles/recipes/claude-mcp/servers.json` and runs via t-man; the web UI is
  `csl web --port 7424` (the `csl-web` t-man agent). There is no `csl serve` —
  `--serve` is a flag on `csl search` that runs the search daemon in foreground.

## Configuration

Config lives at `~/.config/csl/config.yaml`. Key sections:

- **`dirs`** — directories to walk for git repos.
- **`index.hosts`** — allowlist of git remote hosts. Only repos whose origin remote matches a listed host are indexed. Omit to index all repos.
- **`hooks.post_merge.exclude`** — repos to skip during sync/reindex (by absolute path or org/repo name).

`csl sync` discovers repos via `FilteredWalk(cfg.Dirs, cfg.Index.Hosts)`, pulls them (ff-only), and reindexes any that changed. Newly discovered repos that have no entry in `state.json` are also indexed on first sync.

## zoekt query pitfalls

When writing or debugging csl_search queries:

- AND requires all terms in the same file. 3+ space-separated terms almost always return zero results. Use 1-2 terms + filters.
- OR: `|` with no spaces. Uppercase `OR` is literal. `a | b` (with spaces) is three AND terms.
- Filter prefixes: `repo:` (not `r:`), `f:` (not `file:`). The tool also has dedicated `repo`/`lang`/`file` params — prefer those over inline syntax.
- `f:` is regex not glob: `f:.*\.go$` not `f:*.go`.
- Dots in terms trigger regex mode: escape with `\.` for literal dots.
- Unclosed quotes fail with a parse error.
- `csl_query_validate` shows the parsed tree — use it whenever a query returns unexpected results.

## See also

- Recipe: `recipes/csl/recipe.toml` (this repo)
- Shared UI package: `webkit/` at the repo root
