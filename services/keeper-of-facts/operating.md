# operating kof

keeper-of-facts (kof) is an assertion store. Each assertion is a one-sentence
claim about how a system behaves, pinned to evidence: a hashed line range in a
repo working tree. `kof check` re-hashes the pins and flips an assertion stale
when the pinned code changes, so recorded knowledge decays detectably instead
of going quietly wrong.

## how it runs

A single `kof serve` process is the only writer. It owns the store, resolves
and hashes evidence pins, and serves the JSON API plus a read-only web page on
localhost (default {{.BaseURL}}). Everything else is a thin HTTP client over
that API: the CLI commands (`assert`, `list`, `get`, `check`, `retract`,
`recall`) and the MCP server, a stdio shim spawned as `kof mcp`. The shim
being up says nothing about the service: every tool call it handles is a live
HTTP request to serve, and it fails when serve is down. Machines usually also
route http://kof.this to serve via the local domain front door; if the
localhost port answers but the .this host does not, the router is the problem,
not this service.

## where state lives

The store is an append-only JSONL log in the workdir, default {{.StorePath}}
(override with KOF_WORKDIR). Two files: `assertions.jsonl` holds most records;
`local.jsonl` holds records whose subject starts with `machine:` and never
leaves the machine. Every mutation appends one complete record as one line. On
load the newest record per id wins: greater `updated_at`, with a tie going to
the later line. Nothing is rewritten in place, so the files are safe to read
directly and history stays intact.

## failure modes

Connection refused, or "kof serve not reachable": serve is not running. t-man
typically supervises it. Run `t-man list` to see whether the
keeper-of-facts agent exists, then `t-man restart keeper-of-facts`. For a
quick test without t-man, `kof serve` in a spare terminal also works.

Empty query result: usually not an error. `kof_query` and `kof list` match
`subject` as a prefix, so `repo:org/name` finds
`repo:org/name/services/x`, while `services/x` alone finds nothing. Retry
with a shorter prefix, or with no subject filter, before concluding the store
is empty.

Stale assertions: the pinned code changed after the assertion was recorded.
Treat stale as "re-verify before trusting". Hashing covers the exact line
range, so an edit above a pin shifts the lines and flips the assertion stale
even when the pinned code only moved. Check the claim against the current
code, then re-assert with fresh pins if it still holds, or retract it with a
note if it does not.

Assert rejected with a 400: the server refused to ground the evidence. Either
the request had no pins, a pin names a file that does not exist under `repo_path`,
or the line span runs past the end of the file. `repo_path` must be the
absolute path to the repo working tree on this machine.

## version skew

`kof version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: restart it (`t-man restart keeper-of-facts`) and compare again.

## first moves

1. `curl -s -o /dev/null -w '%{http_code}' {{.BaseURL}}/healthz` (204 means serve is up)
2. If unreachable: `t-man list`, then `t-man restart keeper-of-facts`
3. Compare `kof version -o json` with the `/version` endpoint for skew
4. `kof list` with no filters, to confirm the store loads and has records
5. List the workdir, to confirm the two JSONL files exist and are non-empty
