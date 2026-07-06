# Web UI

`csl web` serves a localhost browser UI for code search over the same zoekt
index used by the CLI and MCP server.

```sh
csl web --port 7424     # http://localhost:7424
```

The server binds to `127.0.0.1` only — it is never exposed on the LAN. The
frontend (HTML, CSS, JS) is embedded in the binary, so there is nothing to
install or build separately.

## Pages

- **Search** (`/`) — a **Lexical ⇄ Semantic** mode toggle, a query box, a
  Files ⇄ Matches view toggle, and a row of quick-filter chips. Results are
  grouped by repo, then file. Each file links to its source on the git host;
  each hit can be expanded inline to show surrounding lines. Large result sets
  are paginated: files are revealed in pages with a **Load more** button. When
  the server-side file cap is reached, a note explains the result was truncated
  and suggests narrowing the query.

The empty state (no query) shows a categorized set of example queries. Each
example deep-links to the Search page (`/?q=<query>`) and runs automatically.
The examples differ by mode: lexical shows zoekt query syntax (basics, regular
expressions, boolean operators, scoping, symbol search); semantic shows
plain-language descriptions grouped by intent.

## Lexical vs semantic

The mode toggle picks the backend:

- **Lexical** (default) — zoekt: exact text and regex matching. Same index and
  query syntax as the CLI and the [`csl_search`](mcp.md#csl_search) MCP tool.
- **Semantic** — vector embeddings: matches by meaning, so a plain-language
  description finds code even when the words don't appear in it. Backed by the
  semantic index (`csl index --semantic-all`) and the
  [`csl_semantic_search`](mcp.md#csl_semantic_search) MCP tool. When the index
  is not built, the page shows a hint instead of results.

## Quick-filter chips

Below the query box, a row of chips lets you narrow the visible results without
editing the query. The model is **select-to-show**: nothing is selected by
default (all results show), and clicking a chip filters down to that group.
Click more chips to widen the selection (multi-select); click the last selected
chip to clear the filter. Each chip shows a count.

- In **lexical** mode the chips are file suffixes (Go, YAML, …) and the
  selection is applied server-side as a file-path filter on the query.
- In **semantic** mode the chips are languages, applied client-side to the
  ranked hit list. The chips appear only when the results span two or more
  groups.

The header has a light/dark theme toggle. The default is light; the choice is
remembered in `localStorage`.

## How search works

`csl web` reuses the CLI's search path: it tries the search daemon first
(auto-starting it if needed) and falls back to an in-process search when the
daemon is unavailable. The first search in a fresh checkout triggers an initial
index build. See [Architecture](architecture.md) for the daemon and index
internals.

## Remote links

Each result links to the file on its git host using the `HEAD` ref, which the
host resolves to the repo's default branch:

```
https://<host>/<org>/<repo>/blob/HEAD/<file>#L<line>
```

The host and `org/repo` come from the repo's `origin` remote. Repos without a
remote render without a link.

## JSON API

The UI is backed by a small JSON API on the same port. It is convenient for
scripting and testing.

### `GET /api/search`

| Param | Default | Description |
|-------|---------|-------------|
| `q` | — | Zoekt query (required). |
| `mode` | `files_with_matches` | `files_with_matches` or `content`. |
| `repo` | — | Restrict to repos matching this regex. |
| `lang` | — | Restrict to a language (e.g. `go`). |
| `file` | — | Restrict to file paths matching this regex. |
| `case` | — | `yes` forces case-sensitive matching. |
| `limit` | `50` | Max files returned (capped at 500). |
| `context` | `0` | Context lines per hit in `content` mode (capped at 20). |

Response: matches grouped by repo and file, with `remoteURL` per hit, plus
pagination metadata:

- `total` — number of matches returned.
- `files` — number of distinct files returned.
- `limit` — the file limit applied to this request.
- `truncated` — `true` when `files` reached `limit`, meaning more files may match
  and the result set was capped. Narrow the query (or raise `limit`, up to 500)
  to see more.
- `repos` — always an array (`[]` when there are no matches).

```sh
curl 'http://localhost:7424/api/search?q=func%20Walk&mode=files'
```

```json
{
  "query": "func Walk",
  "mode": "files_with_matches",
  "total": 1,
  "files": 1,
  "limit": 200,
  "truncated": false,
  "repos": [
    {
      "repo": "mad01/thismoon",
      "host": "github.com",
      "files": [
        {
          "file": "internal/repo/finder/walker.go",
          "fileURL": "https://github.com/mad01/thismoon/blob/HEAD/internal/repo/finder/walker.go",
          "matches": [
            {
              "line": 18,
              "column": 6,
              "text": "func Walk(dirs []string) ([]Repo, error) {",
              "remoteURL": "https://github.com/mad01/thismoon/blob/HEAD/internal/repo/finder/walker.go#L18"
            }
          ]
        }
      ]
    }
  ]
}
```

### `GET /api/semantic_search`

| Param | Default | Description |
|-------|---------|-------------|
| `q` | — | Natural-language query (required). |
| `k` | `10` | Max results (capped at 500). |
| `repo` | — | Restrict to repos containing this substring. |
| `lang` | — | Restrict to a single language. |
| `expand` | `0` | Extra source lines above and below each chunk (capped at 20). |

Response: a flat ranked list of hits. `available` is `false` (with a `note`)
when the semantic index has not been built.

```sh
curl 'http://localhost:7424/api/semantic_search?q=retry%20a%20failed%20request&k=5'
```

```json
{
  "available": true,
  "query": "retry a failed request",
  "hits": [
    {
      "repo": "mad01/thismoon",
      "path": "internal/queue/queue.go",
      "lang": "go",
      "kind": "function",
      "start_line": 88,
      "end_line": 121,
      "score": 0.742,
      "snippet": "func (q *Queue) reindexWithRetry(...) { ... }",
      "fileURL": "https://github.com/mad01/thismoon/blob/HEAD/internal/queue/queue.go#L88"
    }
  ]
}
```

### `GET /api/read`

| Param | Default | Description |
|-------|---------|-------------|
| `repo` | — | Repo name (substring match, required). |
| `file` | — | File path relative to the repo root (required). |
| `start` | `0` | First line, 1-based (0 = start of file). |
| `end` | `0` | Last line, 1-based (0 = end of file). |

```sh
curl 'http://localhost:7424/api/read?repo=thismoon&file=internal/cli/root.go&start=1&end=20'
```

### `GET /api/repos`

Lists discovered repos (`name`, `host`, `remote`), filtered by the configured
host allowlist.

### `GET /healthz`

Returns `ok`. Useful for readiness checks when running as a service.

## Running as a service

Register `csl web` with a process manager so the UI is always reachable:

```sh
t-man add --name csl-web -- csl web --port 7424
```
