# wire configuration

## Where config lives

wire has no config file. Configuration is four persistent command-line
flags, shared across `wire serve`, `wire mcp`, and every CLI command, each
with an environment-variable fallback. Precedence is flag, then environment
variable, then built-in default.

## Flags

These are persistent flags on the `wire` root command, so they apply to
`serve`, `mcp`, and every management subcommand (`open`, `join`, `leave`,
`post`, `read`, `follow`, `close`, `list`, `connect`).

- `--workdir` (string, default `~/.local/share/wire`, env `WIRE_WORKDIR`):
  directory holding `channels.jsonl` and `messages/<channel-id>.jsonl`. Only
  `wire serve` writes here; the CLI and MCP read it indirectly through
  serve's HTTP API.
- `--port` (int, default `7432`, env `WIRE_PORT`): port `wire serve` listens
  on, and the port the CLI and `wire mcp` connect to as clients. serve binds
  loopback only (`127.0.0.1:<port>`): nothing authenticates requests, so it
  must stay unreachable from outside the machine.
- `--base-url` (string, default `http://localhost:<port>`, env
  `WIRE_BASE_URL`): the human-facing URL the MCP tools return alongside a
  channel's connection string (e.g. `http://wire.this/?channel=<name>`).
  Purely cosmetic: it doesn't change what host or port the client actually
  connects to, which is always `--port`.
- `--from` (string, default `WIRE_FROM` env var, else the OS username, env
  `WIRE_FROM`): the name CLI-posted messages are signed with. The MCP tools
  take `from` as an explicit call argument instead of reading this flag.

A leading `~` in `--workdir` is expanded at runtime, since `WIRE_WORKDIR`
reaches Go without shell expansion.

## Environment variables

- `WIRE_WORKDIR`: fallback for `--workdir`.
- `WIRE_PORT`: fallback for `--port`.
- `WIRE_BASE_URL`: fallback for `--base-url`.
- `WIRE_FROM`: fallback for `--from`.

## Example

```bash
export WIRE_PORT=7432
export WIRE_WORKDIR=~/.local/share/wire
export WIRE_BASE_URL=http://wire.this
export WIRE_FROM=planner

wire serve --port "$WIRE_PORT" --workdir "$WIRE_WORKDIR"
```

Everything else, including which channels exist, who's on the roster, and
what's been said, lives in the append-only JSONL store under `--workdir`,
not in any config.
