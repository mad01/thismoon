# keeper-of-facts configuration

## Where config lives

keeper-of-facts (kof) has no config file. It resolves its settings from
persistent CLI flags on the `kof` command, each with an environment variable
fallback and a compiled-in default. Precedence is flag over environment
variable over default. The flags are persistent, so they apply to every
subcommand (`serve`, `mcp`, `assert`, `list`, ...), though only `serve` and
`mcp` read them for anything beyond talking to a running `kof serve`.

Every `KOF_*` variable also accepts its pre-rename `KEEP_*` spelling as a
fallback, checked after the `KOF_*` name, so a machine mid-way through the
`keep` to `keeper-of-facts` cutover keeps working. New setups should use the
`KOF_*` names; the `KEEP_*` fallback exists only for migration.

## Flags

- `--workdir` (string, default `~/.local/share/kof`, env `KOF_WORKDIR`,
  legacy env `KEEP_WORKDIR`): directory holding the assertion log
  (`assertions.jsonl` and `local.jsonl`). Only `kof serve` reads this, since
  it is the sole writer. A leading `~` expands to the user's home directory
  at startup. On start, `serve` also auto-migrates a populated pre-rename
  `~/.local/share/keep` store into this directory if one exists; it refuses
  to start if both stores hold data, rather than guess which one is current.
- `--port` (int, default `7431`, env `KOF_PORT`, legacy env `KEEP_PORT`): the
  port `kof serve` listens on, and the port every other command (the MCP
  server, the CLI's `assert`/`list`/`get`/`check`/`retract`/`recall`)
  connects to as an HTTP client. `serve` binds to `127.0.0.1` only:
  assertions are personal and the API has no authentication, so it must
  never be reachable off localhost.
- `--base-url` (string, default `http://localhost:<port>`, env
  `KOF_BASE_URL`, legacy env `KEEP_BASE_URL`): the human-facing URL the MCP
  server returns in tool responses (for example `http://kof.this`, when a
  local domain front door routes to the service). This is display-only: the
  MCP client itself always calls `localhost:<port>` regardless of this
  value.

A `~` that cannot be expanded — no resolvable home directory, which happens
under a launchd agent with a stripped environment — is an error at startup.
Earlier releases stripped the `~` and used a path relative to the working
directory instead, which quietly served an empty store. This applies to
`--pin`'s `repo_path` as well. An unparseable `KOF_PORT` warns once on stderr
and falls back to the compiled-in default.

To see what a given environment actually resolved to, run `kof doctor`; an
agent with no shell gets the same report from the `kof_doctor` MCP tool.

## Environment variables

- `KOF_AUTHOR` (default: the OS username, legacy env `KEEP_AUTHOR`): the
  identity `kof serve` stamps into `provenance.author` on every assertion it
  creates. Read only by `serve`, and only at the moment it constructs its
  handler, not per request: every assertion recorded by a given `serve`
  process carries the same author. There is no matching flag; set it in the
  environment the `serve` process runs under (for example the `t-man add`
  invocation that supervises it).

## Example

kof is flag/env-only, so there is no config file to show. A typical
supervised launch looks like:

```bash
KOF_AUTHOR=alexander \
  kof serve --port 7431 --workdir ~/.local/share/kof --base-url http://kof.this
```
