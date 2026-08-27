# keeper-of-facts (kof), assertion store with evidence pins

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, all over
one append-only JSONL store. An agent session deposits a one-sentence assertion about how a
system behaves and pins it to evidence: a line range in a repo working tree,
hashed the moment the session records it. `kof check` re-hashes those pins and flips
an assertion stale when the pinned code has changed. **The MCP tools (driven by
Claude) are the primary surface**; the `kof` CLI mirrors them. The web page at
`http://kof.this/` is a **read-only** view of the same store.

## Module layout

```
keeper-of-facts/
  cmd/kof/            - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra: root, serve, mcp, manage (assert/list/recall/get/check/retract), version (build metadata from the shared buildinfo package)
    client/            - HTTP client shared by the CLI and mcpserver (client.go)
    store/             - Assertion model + pure helpers (assertion.go) and the
                         mutex-guarded JSONL store (store.go, id.go)
    pin/               - Pin model + working-tree hashing (pin.go): resolve, re-hash, compare
    recall/            - one-shot `claude -p` judge that ranks the whole store against a free-form question (recall.go)
    server/            - HTTP API + webkit web page (embedded shell.html + app.js)
    mcpserver/         - MCP tools (server.go = MCP server setup, tools.go = 6 tools)
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Single-writer architecture (load-bearing)

Assertions are mutated from three places (the MCP, the CLI, and `kof check`),
so to avoid two processes racing on the JSON file, **`kof serve` is the only
writer**:

- **`kof serve`** owns the store (in-memory map guarded by a mutex,
  persisted to the JSONL log under `~/.local/share/kof/`). It runs the HTTP
  server (web page + JSON API + `/version` + `/webkit/`).
- **`kof mcp`** holds no state: it is a thin HTTP client to the serve API on
  `localhost:<port>`. If serve is down, tools return "kof serve not reachable
  … (t-man status keeper-of-facts)".
- **`kof` CLI** mutations (`assert`, `check`, `retract`) are the same thin
  HTTP client.

So the MCP, the CLI, and the web page all see the same data, and there are no
file locks. Pin resolution and hashing happen inside serve, which is the process
with a coherent view of the working tree the pins point at.

The one sanctioned exception: serve polls the log files every 2 seconds and
reloads the store when another **process** changed them on disk (a git pull of
a synced workdir, a manual append). The reload never repairs the files and
keeps the old in-memory data on any parse error. kof processes themselves
still never write concurrently — serve stays the single kof writer.

## Data model & storage

```go
Assertion{
  ID,                // time-sortable
  Kind,              // code-behavior | dead-end | preference | decision | machine-state | open-thread
  Subject,           // namespaced key, e.g. repo:mad01/thismoon/services/events
  Statement,         // one sentence
  Pins,              // >= 1 required
  Confidence,        // verified | derived | hint
  Provenance{ Author, SessionID, DerivedAt, CostTokens },
  Status,            // fresh | stale | retracted
  StaleReason,       // set when Status is stale
  RetractNote,       // set when Status is retracted
  CheckedAt,
  Links,
}

Pin{
  RepoPath,          // absolute path to the repo working tree
  Repo,              // canonical host/org/name from the origin remote ("" when none)
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
- serve stamps **Author** at assert time (`KOF_AUTHOR`, else the OS username)
  and never takes it from the request. serve likewise derives `Pin.Repo` from
  the checkout's origin remote; both are the machine-independent identity a
  shared store will need, while resolution still goes via `repo_path`.
- Every assertion needs at least one pin. An assertion with zero pins can't go
  stale, so kof refuses to store one.

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
serve auto-migrates a pre-rename `~/.local/share/keep` store to `~/.local/share/kof` on start (MAD-269; two populated stores refuse loudly). Workdir defaults to `~/.local/share/kof` and is overridable with
`KOF_WORKDIR`.

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
make build    # ./kof binary (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/kof + adhoc codesign
make test     # go test ./...  (hermetic: t.TempDir + a temp git repo, never touches real $HOME)
```

## HTTP API

Owned by `kof serve`:

- `GET  /`                                : webkit-chromed web page, read-only list with filters
- `GET  /app.js`                          : the page's client script
- `GET  /api/assertions?subject=&kind=&status=` : list, newest first; `subject` is a prefix match
- `POST /api/assertions`                  : create; server resolves and hashes the pins (0 pins or a resolve failure → 400)
- `GET  /api/assertions/{id}`             : one assertion, full detail
- `POST /api/assertions/{id}/retract`     : body `{note}`; terminal withdrawal
- `POST /api/check`                        : body `{id?}`; check one assertion or all → `{checked, fresh, stale, flipped, assertions}`
- `POST /api/recall`                       : body `{question}`; model-judged relevance over the whole store, ranked, capped at 5 (502 naming the kof_query fallback when the judge fails)
- `GET  /healthz`                         : 204
- `GET  /version`                          → the four-key build metadata object (`version`, `commit`, `tag`, `build_time`)
- `GET  /webkit/`                         : shared chrome (asset drift checks use `GET /webkit/version`)

Pin resolution lives on the server because it is the process that can read the
working tree the pins name. A `POST /api/assertions` with zero pins, or with a
pin that names a missing file or an out-of-range line span, is a 400: kof never
stores an assertion it couldn't ground.

## Commands

CLI surface beyond `serve`/`mcp`, wired as thin HTTP clients to `kof serve`
(`internal/client`):

```bash
kof serve --port 7431 --workdir ~/.local/share/kof
kof mcp
kof assert --kind <kind> --subject <key> --statement <text> \
            --confidence verified|derived|hint --session <id> \
            [--cost-tokens <n>] [--link <url>]... \
            --pin <repo_path>:<file>:<start>-<end>   # repeatable, at least one
kof list [--subject <prefix>] [--kind <kind>] [--status fresh|stale|retracted]
kof recall "<question>"                     # model-judged relevance over the whole store
kof get <id>
kof check [id]                             # one assertion, or all when id is omitted
kof retract <id> --note <reason>
kof version [-o json]
```

`--pin` takes `repo_path:file:start-end` and repeats; at least one is required,
and `repo_path` is the absolute path to the repo working tree.
`kof check` with no id walks the whole store (skipping retracted ones) and
prints how many flipped. `kof version` prints the bare git commit that built
the binary, the token sibling tools also print so ralph and status can probe any
of them for the build they are running; `-o json` prints the full build
metadata object.

## MCP tools

Thin client over the API above (`internal/client`), served on stdio by
`kof mcp`:

- `kof_assert(kind, subject, statement, confidence, session_id, pins, cost_tokens?, links?)`: create; `pins` is a list of objects (`repo_path` — absolute path to the working tree, `file`, `start_line`, `end_line`), at least one
- `kof_query(subject?, kind?, status?)`: list, newest first; `subject` is a prefix match
- `kof_recall(question)`: ask the keeper what it knows relevant to a free-form question — a one-shot isolated `claude -p` haiku judge (`internal/recall`) ranks the whole store and returns the relevant assertions in rank order; no embeddings (MAD-265). Judge failure errors name the kof_query fallback. Needs `claude` on serve's PATH.
- `kof_get(id)`: one assertion, full detail
- `kof_retract(id, note)`: terminal withdrawal with a counter-evidence note
- `kof_check(id?)`: re-hash one assertion's pins, or all when `id` is omitted; returns the fresh/stale/flipped counts

Tool responses include `url` (the human-facing `KOF_BASE_URL`, e.g.
`http://kof.this`), while the client itself calls `localhost:<KOF_PORT>`. Both
env vars pin where the MCP looks, the same way reminder's do.

## Shared UI: webkit

The web page chrome (`<wk-header>` + theme/font/size controls) and components
(`<wk-card>`, `<wk-badge>`, `<wk-page-header>`, `<wk-title>`) come from the
in-module package **`github.com/mad01/thismoon/webkit`**, mounted at `GET
/webkit/` via `webkit.Mount(mux)` and loaded by `internal/server/shell.html`
(which pulls the FOUC guard from `/webkit/boot.js`). Don't re-add
palette/topbar/theme CSS locally; it lives in webkit only.

kof's web page is **read-only**: it lists assertions with filters for subject,
kind, and status, and shows each one's pins and current state. It makes no
mutations. Writing an assertion, checking it, and retracting it all go through
the MCP or the CLI. kof is a webkit consumer like the other services in this
repo: no pin or bump step, so a webkit change ships at the next build.

```bash
make install && t-man restart keeper-of-facts
```

Confirm the shared assets with `GET /webkit/version`: every consumer built from
the same commit reports the same asset hash.

## Gotchas

- **Wave 0 builder.** Builds before the consuming repo's `claude-mcp` recipe (wave 1) registers the MCP.
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `kof docs`): serve-must-be-running, store layout, failure modes,
  version-skew checks. Keep those facts there, not here.
- **Codesign for the binary.** `make install` strips xattrs and re-signs (macOS
  kills adhoc-signed binaries with drifted provenance).
- **Version probe convention.** `GET /version` and `kof version -o json` both
  return the shared four-key build metadata object (`version`, `commit`, `tag`,
  `build_time`, every key present and `""` when unknown) from
  `github.com/mad01/thismoon/buildinfo`, so ralph can check which build is live.
  Plain `kof version` stays a bare token — status parses it as one.

## See also

- Recipe: `recipes/keeper-of-facts/recipe.toml` (+ `recipes/keeper-of-facts/CLAUDE.md`)
- Human docs: `README.md`
- The `kof.this` d-man route and the `claude-mcp` `servers.json` MCP
  registration live in the consuming repo's private overlay (docs/adr/0006),
  not here.
