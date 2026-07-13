# keep, assertion store with evidence pins

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, all over
one append-only JSONL store. An agent session deposits a one-sentence assertion about how a
system behaves and pins it to evidence: a line range in a repo working tree,
hashed the moment the session records it. `keep check` re-hashes those pins and flips
an assertion stale when the pinned code has changed. **The MCP tools (driven by
Claude) are the primary surface**; the `keep` CLI mirrors them. The web page at
`http://keep.this/` is a **read-only** view of the same store.

## Module layout

```
keep/
  cmd/keep/            - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra: root, serve, mcp, manage (assert/list/get/check/retract), version (Version via ldflags)
    client/            - HTTP client shared by the CLI and mcpserver (client.go)
    store/             - Assertion model + pure helpers (assertion.go) and the
                         mutex-guarded JSONL store (store.go, id.go)
    pin/               - Pin model + working-tree hashing (pin.go): resolve, re-hash, compare
    server/            - HTTP API + webkit web page (embedded shell.html + app.js)
    mcpserver/         - MCP tools (server.go = MCP server setup, tools.go = 5 tools)
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Single-writer architecture (load-bearing)

Assertions are mutated from three places (the MCP, the CLI, and `keep check`),
so to avoid two processes racing on the JSON file, **`keep serve` is the only
writer**:

- **`keep serve`** owns the store (in-memory map guarded by a mutex,
  persisted to the JSONL log under `~/.local/share/keep/`). It runs the HTTP
  server (web page + JSON API + `/version` + `/webkit/`).
- **`keep mcp`** holds no state: it is a thin HTTP client to the serve API on
  `localhost:<port>`. If serve is down, tools return "keep serve not reachable
  … (t-man status keep)".
- **`keep` CLI** mutations (`assert`, `check`, `retract`) are the same thin
  HTTP client.

So the MCP, the CLI, and the web page all see the same data, and there are no
file locks. Pin resolution and hashing happen inside serve, which is the process
with a coherent view of the working tree the pins point at.

## Data model & storage

```go
Assertion{
  ID,                // time-sortable
  Kind,              // code-behavior | dead-end | preference | decision | machine-state | open-thread
  Subject,           // namespaced key, e.g. repo:mad01/thismoon/services/events
  Statement,         // one sentence
  Pins,              // >= 1 required
  Confidence,        // verified | derived | hint
  Provenance{ SessionID, DerivedAt, CostTokens },
  Status,            // fresh | stale | retracted
  StaleReason,       // set when Status is stale
  RetractNote,       // set when Status is retracted
  CheckedAt,
  Links,
}

Pin{
  RepoPath,          // absolute path to the repo working tree
  File,              // path within the repo
  StartLine, EndLine, // 1-based inclusive
  ContentSHA256,     // sha256 of the line range read from the working tree
  HeadCommit,        // the commit HEAD pointed at when the pin resolved
  ResolvedAt,
}
```

- **Kinds** name what a session learned: `code-behavior` (how code acts),
  `dead-end` (an approach that failed), `preference` (a user's stated
  preference), `decision` (a choice and its reason), `machine-state` (a fact
  about the local host), `open-thread` (a question left unanswered).
- **Confidence** is the session's own grading: `verified` (checked against a
  primary source), `derived` (reasoned from evidence), `hint` (a weak signal
  worth recording).
- Every assertion needs at least one pin. An assertion with zero pins can't go
  stale, so keep refuses to store one.

Persisted as an append-only JSONL log: every mutation appends one complete
record as one line, and load resolves the newest record per id (greater
`updated_at` wins; a tie goes to the later line). Two files under the workdir:

- `assertions.jsonl` — everything except machine-scoped records; the file
  federation will sync (MAD-246).
- `local.jsonl` — records whose subject starts with `machine:`; never leaves
  the machine. Routing happens at write time and subjects are immutable, so a
  record never moves between files.

A pre-JSONL `assertions.json` array is migrated on startup (one line per
record, oldest first) and renamed to `assertions.json.migrated` as a backup.
Workdir defaults to `~/.local/share/keep` and is overridable with
`KEEP_WORKDIR`.

### State transitions

| From | Event | To | Notes |
|------|-------|-----|-------|
| (none) | `assert` | `fresh` | server resolves and hashes every pin first |
| `fresh` | `check`, a pin's content differs | `stale` | `stale_reason` names the first failing pin |
| `stale` | `check`, all pins hash the same again | `fresh` | stale is reversible |
| `fresh`/`stale` | `retract(note)` | `retracted` | terminal; `check` skips it |
| `retracted` | `retract(note)` | `retracted` | idempotent no-op |

There is no un-retract and no delete in v1. A retracted assertion stays in the
store as a record that the claim was withdrawn, with the counter-evidence note
explaining why.

### Check semantics and a known limitation

`check` re-reads each pin's line range from the working tree, hashes the raw
bytes (newlines included), and compares against the stored `content_sha256`.
When every pin still matches, the assertion is fresh. When one doesn't, the
assertion goes stale and `stale_reason` records the first failing pin, e.g.
`content changed (main.go:10-20)`.

Hashing is over the line range, not the surrounding code, so it is conservative
by design: an edit **above** a pin shifts its lines down and flips the assertion
stale even when the pinned code itself only moved. A stale assertion means "the
evidence needs another look", not "the claim is wrong". Re-asserting with fresh
pins is the way to record the moved evidence.

## Build / install / test

```bash
make build    # ./keep binary (Version via ldflags)
make install  # build + cp to ~/code/bin/keep + adhoc codesign
make test     # go test ./...  (hermetic: t.TempDir + a temp git repo, never touches real $HOME)
```

## HTTP API

Owned by `keep serve`:

- `GET  /`                                : webkit-chromed web page, read-only list with filters
- `GET  /app.js`                          : the page's client script
- `GET  /api/assertions?subject=&kind=&status=` : list, newest first; `subject` is a prefix match
- `POST /api/assertions`                  : create; server resolves and hashes the pins (0 pins or a resolve failure → 400)
- `GET  /api/assertions/{id}`             : one assertion, full detail
- `POST /api/assertions/{id}/retract`     : body `{note}`; terminal withdrawal
- `POST /api/check`                        : body `{id?}`; check one assertion or all → `{checked, fresh, stale, flipped, assertions}`
- `GET  /healthz`                         : 204
- `GET  /version`                          → `{"version":"<sha>"}`
- `GET  /webkit/`                         : shared chrome (asset drift checks use `GET /webkit/version`)

Pin resolution lives on the server because it is the process that can read the
working tree the pins name. A `POST /api/assertions` with zero pins, or with a
pin that names a missing file or an out-of-range line span, is a 400: keep never
stores an assertion it couldn't ground.

## Commands

CLI surface beyond `serve`/`mcp`, wired as thin HTTP clients to `keep serve`
(`internal/client`):

```bash
keep serve --port 7431 --workdir ~/.local/share/keep
keep mcp
keep assert --kind <kind> --subject <key> --statement <text> \
            --confidence verified|derived|hint --session <id> \
            [--cost-tokens <n>] [--link <url>]... \
            --pin <repo_path>:<file>:<start>-<end>   # repeatable, at least one
keep list [--subject <prefix>] [--kind <kind>] [--status fresh|stale|retracted]
keep get <id>
keep check [id]                             # one assertion, or all when id is omitted
keep retract <id> --note <reason>
keep version [-o json]
```

`--pin` takes `repo_path:file:start-end` and repeats; at least one is required,
and `repo_path` is the absolute path to the repo working tree.
`keep check` with no id walks the whole store (skipping retracted ones) and
prints how many flipped. `keep version` prints the git commit that built the
binary; `-o json` prints `{"version":"<sha>"}`, the convention sibling tools
follow so ralph can probe any of them for the build they are running.

## MCP tools

Thin client over the API above (`internal/client`), served on stdio by
`keep mcp`:

- `keep_assert(kind, subject, statement, confidence, session_id, pins, cost_tokens?, links?)`: create; `pins` is a list of objects (`repo_path` — absolute path to the working tree, `file`, `start_line`, `end_line`), at least one
- `keep_query(subject?, kind?, status?)`: list, newest first; `subject` is a prefix match
- `keep_get(id)`: one assertion, full detail
- `keep_retract(id, note)`: terminal withdrawal with a counter-evidence note
- `keep_check(id?)`: re-hash one assertion's pins, or all when `id` is omitted; returns the fresh/stale/flipped counts

Tool responses include `url` (the human-facing `KEEP_BASE_URL`, e.g.
`http://keep.this`), while the client itself calls `localhost:<KEEP_PORT>`. Both
env vars pin where the MCP looks, the same way reminder's do.

## Shared UI: webkit

The web page chrome (`<wk-header>` + theme/font/size controls) and components
(`<wk-card>`, `<wk-badge>`, `<wk-page-header>`, `<wk-title>`) come from the
in-module package **`github.com/mad01/thismoon/webkit`**, mounted at `GET
/webkit/` via `webkit.Mount(mux)` and loaded by `internal/server/shell.html`
(which pulls the FOUC guard from `/webkit/boot.js`). Don't re-add
palette/topbar/theme CSS locally; it lives in webkit only.

keep's web page is **read-only**: it lists assertions with filters for subject,
kind, and status, and shows each one's pins and current state. It makes no
mutations. Writing an assertion, checking it, and retracting it all go through
the MCP or the CLI. keep is a webkit consumer like the other services in this
repo: no pin or bump step, so a webkit change ships at the next build.

```bash
make install && t-man restart keep
```

Confirm the shared assets with `GET /webkit/version`: every consumer built from
the same commit reports the same asset hash.

## Gotchas

- **Wave 0 builder.** Builds before `recipes/claude-mcp` (wave 1) registers the MCP.
- **serve must be running for the MCP/CLI to work**: it owns the store and does
  the pin hashing. It runs as a t-man agent; `t-man status keep` /
  `t-man restart keep`.
- **check is conservative.** It hashes the pinned line range, so an edit above a
  pin shifts the lines and flips the assertion stale even though the content
  only moved. Stale means "re-verify", not "wrong". Re-assert with fresh pins.
- **Codesign for the binary.** `make install` strips xattrs and re-signs (macOS
  kills adhoc-signed binaries with drifted provenance).
- **Version probe convention.** `GET /version` and `keep version -o json` both
  return `{"version":"<sha>"}` so ralph can check which build is live.

## See also

- Recipe: `recipes/keep/recipe.toml` (+ `recipes/keep/CLAUDE.md`)
- Human docs: `README.md`
- The `keep.this` d-man route and the `claude-mcp` `servers.json` MCP
  registration live in the consuming repo's private overlay (docs/adr/0006),
  not here.
