# moondb configuration

## Where config lives

moondb resolves each setting from a flag, then an environment variable, then
the config file, then a compiled-in default. The flags are persistent, so
every subcommand takes them. `serve` binds the port, `record` writes under
the workdir, and every other command uses the port to reach serve.

The config file is TOML and optional:

1. `--config <path>`
2. `$MOONDB_CONFIG`
3. `$XDG_CONFIG_HOME/moondb/config.toml`, when that variable holds an
   absolute path
4. `~/.config/moondb/config.toml` (default)

A leading `~` is expanded before the file is opened. A home directory that
cannot be resolved is an error, never a path relative to the working
directory. That matters most for `record`, which runs inside a hook with
whatever environment Claude Code gives it.

A missing file is not an error: the defaults apply. A file that is present
but malformed fails closed. `serve` refuses to start and names the key.
`record` captures nothing, writes one line to stderr (never stdout), and
still exits 0, so the session carries on. `moondb doctor` reports the parse
error. Nothing captures with settings nobody could read, because a broken
`capture.exclude` list could otherwise store a file the user meant to keep
out.

## Flags

- `--port` (int, default `7433`, env `MOONDB_PORT`, config `port`): the port
  `moondb serve` listens on, and the port every other command connects to.
  `serve` binds `127.0.0.1` only: the store holds prompts and source code and
  the API has no authentication.
- `--workdir` (string, default `~/.local/state/moondb`, env
  `MOONDB_WORKDIR`, config `workdir`): the state directory. It holds `log/`,
  `objects/`, `spool/` and `sessions/`. `record`, `import` and `serve` must
  resolve the same workdir, or the spool they write is not the one serve
  drains.
- `--base-url` (string, default `http://localhost:<port>`, env
  `MOONDB_BASE_URL`, config `base_url`): the human-facing URL the MCP server
  returns in tool responses, for example `http://moondb.this`. It is
  display-only: the MCP client always calls `localhost:<port>`.
- `--config` (string, default `~/.config/moondb/config.toml`, env
  `MOONDB_CONFIG`): the config file described above.

An unparseable `MOONDB_PORT` warns once on stderr and falls back to the next
source.

The scorecard reads one loopback service, set by environment only and read
by `serve`:

- `EVENTS_BASE_URL` (default `http://127.0.0.1:7430`): the events service,
  for the belt hints and the belt ledger.

## Config file keys

- `capture.enabled` (bool, default `true`): set it to `false` to turn capture
  off on this machine while leaving the hooks wired. `record` then exits 0
  without writing anything, and the doctor's `hooks-wired` check passes
  with a note.
- `claude_settings` (string, default `~/.claude/settings.json`): the Claude
  Code settings file the doctor's `hooks-wired` check reads. It reports
  each hook entry from "Hook wiring" below that does not run `moondb
  record`, and fails when the file is missing.
- `capture.exclude` (list of globs): paths whose contents are never stored,
  by `record` or by `import`. An excluded file's edit is recorded by path
  only, with no content, patch or observed bytes. A glob matches the file's
  base name or its path relative to the worktree. Setting the key replaces
  the default list, so start from the defaults and add to them:

  ```
  .env  .env.*  *.pem  *.key  id_*  *.p12  *.tfvars  *.tfstate
  credentials*.json  kubeconfig  .npmrc
  ```
- `prices."<model id>"` (table): the price of one model in USD per million
  tokens, used by `moondb trace`, the `moondb_trace` tool and the session
  page. A table sets the rates of its model id. When the id matches a
  built-in row (exactly or by family, as below), it keeps that row's
  long-prompt tier and its fast-mode and US-only multipliers, so new rates
  for Opus 5.5 still price fast mode. An id no built-in row matches is added
  with its rates alone. Only `serve` reads it, because serve computes the
  traces.

  | Key | Required | Default |
  |---|---|---|
  | `input` | yes | |
  | `output` | yes | |
  | `cache_write_5m` | no | 1.25 times `input` |
  | `cache_write_1h` | no | 2 times `input` |
  | `cache_read` | no | 0.1 times `input` |

  A table without `input` or `output`, or with a rate that is negative,
  `nan` or `inf`, fails closed: `serve` refuses to start and names the table
  and the key, and `moondb doctor` reports it. `record` is not affected.

## Prices

The built-in table (`DefaultPrices` in `facts.go`) holds the list prices of
the Claude models, from the Anthropic pricing page
(`platform.claude.com/docs/en/about-claude/pricing`), read on 2026-10-25. A
transcript's model id matches its row exactly first, then by family: a
dated snapshot (`claude-opus-4-5-20251101`), a provider version (`@...`) or
a context marker (`[1m]`) after a row's id finds that row. A provider prefix
such as `us.anthropic.` is dropped first. A new version is not in an older
row's family: `claude-opus-5-6` gets no price from `claude-opus-5`.

Some rows carry more than one rate. Haiku 5.5 bills a call whose prompt
passes 100,000 tokens at its long-prompt rates. Opus 5.5, Opus 5 and Opus
4.8 bill a call made in fast mode at twice their rates. Fast mode on any
other model has no price. A call whose usage reports `inference_geo` "us"
(US-only inference) bills 1.1 times every rate on Claude 4.6 and later
models, on top of fast mode and the long-prompt rates. Earlier models have
no US-only price and bill such a call at their standard rates.

A model with no price, from the table or the config, keeps its tokens and
gets no cost. The session listing and the trace both name it as unpriced,
and the cost covers the priced calls only. It is never reported as zero.
A turn whose calls billed no token has no cost in either.

## Fixed limits

These are compiled into the binary (`facts.go`) and not configurable:

| Limit | Value | Effect |
|---|---|---|
| record budget | 200 ms | `record` spools what it has gathered when the budget runs out |
| Bash budget (`BashBudget`) | 2 s | the budget of the async Bash hook; the files it does not reach are skipped, counted on the turn as `timeout` with no edit |
| snapshot budget | 2 s | the turn-edge snapshots of one hook; a snapshot cut short is marked truncated and counts the rest as timeout |
| snapshot file cap | 1000 | paths one snapshot lists, the session's touched paths first; the rest count as over the file cap |
| status cap | 4 MiB | `git status` output a snapshot or the Bash clean check reads; a worktree with more gets no snapshot, and its Bash files are skipped as `failed` |
| largest chunk | 1 MiB | a larger file is recorded by path and patch, without content; a snapshot lists it by size |
| transcript tail | 4 MiB | the most transcript a turn keeps; a trace of a longer turn sees only its tail |
| log segment | 64 MiB | the size at which `log/` rolls to a new file |
| trace cache | 512 turns | turn traces serve keeps in memory, the least recently read dropped first |
| trace rankings | 10 | spans `moondb trace` and `moondb_trace` list as the longest tool calls and the most expensive model calls |
| idle gap (`IdleGapAfter`) | 30 min | the longest stretch inside a turn with no transcript line that a trace gives to model, tool or harness time. The rest of a longer one is idle time, which active time leaves out |
| totals backfill batch (`TotalsBackfillTurns`) | 20 | turns one totals backfill batch tries, computed or not, and sessions it fills a missing prompt line for |
| totals backfill pace (`TotalsBackfillEvery`) | 1 s | how often the totals backfill runs a batch while the spool is empty |
| session idle (`SessionIdleAfter`) | 10 min | how long a session goes without a turn start, turn end, edit or agent stop before the listing calls it ended |
| open turn stale (`OpenTurnStaleAfter`) | 12 h | how long an open last turn keeps an otherwise idle session running |
| prompt line (`SessionPromptLineRunes`) | 100 runes | the cut of the prompt line a session keeps for its title and a trace shows per turn |
| scorecard peer timeout (`PeerTimeout`) | 2 s | how long the scorecard waits for one answer from the events service; past it, that part is `unavailable` |
| large read (`LargeReadLines`) | 300 lines | a Read of a whole file that returned at least this many lines counts as large |
| scorecard flags (`Flag*`) | see `facts.go` | the count at which a pattern becomes a flag: 1 large read, 3 zero-result searches, 3 searches cut at the limit, 2 failed searches, 5 failed tool calls, 1 compaction, 1 stop failure, 1 permission denial |
| scorecard tables (`ScorecardTopTools`) | 10 | tools and agents `moondb scorecard` lists; `--json` has them all |

## Hook wiring

The hooks live in the Claude Code settings, which the consuming repo owns.
`record` reads the hook event from the payload, so every line runs the same
command:

```json
{
  "hooks": {
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }],
    "PostToolUse": [
      { "matcher": "Edit|Write|MultiEdit|NotebookEdit",
        "hooks": [{ "type": "command", "command": "moondb record", "timeout": 3 }] },
      { "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }
    ],
    "Stop": [{ "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }],
    "SubagentStop": [{ "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }]
  },
  "bashEditDiffEnabled": true
}
```

`bashEditDiffEnabled` is a user or managed setting (Claude Code 2.1.269 or
later). It makes Claude Code add `bashEditDiff` to the Bash payload: up to
five file diffs and up to 200 changed paths. Without it, the Bash hook
records nothing. The Bash line is async: a command has already taken its
time, and the session should not wait on `record` after it.

The diff covers only the repository of the session's working directory, so
a session started in the main checkout never captures Bash edits made in a
linked worktree. It also lists the files a `git pull`, `git checkout` or
`git reset` rewrote. `record` runs one `git status` over the listed files
and records none that is clean afterwards. The turn counts those as
skipped, beside the files past the Bash budget and the ones whose status
git could not give.

## Example

```toml
port = 7433
workdir = "~/.local/state/moondb"
base_url = "http://moondb.this"

[capture]
enabled = true
exclude = [
  ".env", ".env.*", "*.pem", "*.key", "id_*", "*.p12",
  "*.tfvars", "*.tfstate", "credentials*.json", "kubeconfig", ".npmrc",
  "secrets/*",
]

# A model the built-in table lacks; the cache rates derive from input.
[prices."claude-nova-9"]
input = 3
output = 15
```

To see what a given environment resolved to, run `moondb doctor`; an agent
with no shell gets the same report from the `moondb_doctor` MCP tool.
